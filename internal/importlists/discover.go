package importlists

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// DiscoverItem is an aggregated, library-annotated item shown in the Discover UI.
type DiscoverItem struct {
	*ImportListItem
	ListName  string            `json:"list_name"`
	InLibrary bool              `json:"in_library"`
	Defaults  *DiscoverDefaults `json:"defaults,omitempty"`
}

// DiscoverDefaults exposes only add settings, never source credentials.
type DiscoverDefaults struct {
	LibraryPath      string      `json:"library_path"`
	QualityProfileID string      `json:"quality_profile_id"`
	MonitorType      MonitorType `json:"monitor_type"`
	SearchOnAdd      bool        `json:"search_on_add"`
}

// DiscoverItems returns deduplicated items from enabled discover-mode lists for
// the given media type ("movie" or "series"), each annotated with whether it is
// already in the library.
func (m *SyncManager) DiscoverItems(ctx context.Context, mediaType string) ([]*DiscoverItem, error) {
	rows, err := m.store.ListDiscoverItems(ctx, mediaType)
	if err != nil {
		return nil, err
	}

	// Build a library-membership lookup for series (movies use direct lookups).
	var seriesTMDB map[string]bool
	if mediaType == string(MediaTypeSeries) && m.seriesSvc != nil {
		seriesTMDB = map[string]bool{}
		if all, err := m.seriesSvc.ListSeries(ctx); err == nil {
			for _, s := range all {
				if s.TMDBID == nil {
					continue
				}
				id := strings.TrimSpace(*s.TMDBID)
				if id != "" && id != "0" {
					seriesTMDB[id] = true
				}
			}
		} else {
			m.logger.Warn("import-lists: discover list series failed", "error", err)
		}
	}

	seen := map[string]bool{}
	defaults := map[string]*DiscoverDefaults{}
	var out []*DiscoverItem
	for _, row := range rows {
		item := row.Item
		oldID := item.TMDbID
		m.resolveMovieIdentity(ctx, item, row.MediaType)
		if item.TMDbID != oldID {
			if err := m.store.UpsertItem(ctx, item); err != nil {
				m.logger.Error("import-lists: cache resolved movie identity failed", "item_id", item.ID, "error", err)
			}
		}
		key := dedupeKey(item)
		if seen[key] {
			continue
		}
		seen[key] = true

		di := &DiscoverItem{ImportListItem: item, ListName: row.ListName}
		if _, loaded := defaults[item.ListID]; !loaded {
			list, err := m.store.Get(ctx, item.ListID)
			if err != nil {
				return nil, err
			}
			if list == nil {
				return nil, fmt.Errorf("parent list not found")
			}
			defaults[item.ListID] = &DiscoverDefaults{list.LibraryPath, list.QualityProfileID, list.MonitorType, list.SearchOnAdd}
		}
		di.Defaults = defaults[item.ListID]
		di.InLibrary = m.itemInLibrary(ctx, row.MediaType, item, seriesTMDB)
		out = append(out, di)
	}
	return out, nil
}

// TrackDiscoverItem records a completed manual add. Unlike AddDiscoverItem this
// never creates media or reapplies the list's settings. A failed tracking write
// can safely be retried after the library add has succeeded.
func (m *SyncManager) TrackDiscoverItem(ctx context.Context, itemID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, err := m.store.GetItem(ctx, itemID)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("discover item not found")
	}
	list, err := m.store.Get(ctx, item.ListID)
	if err != nil {
		return err
	}
	if list == nil {
		return fmt.Errorf("parent list not found")
	}
	mediaType := item.MediaType
	if mediaType == "" {
		mediaType = string(list.MediaType)
	}
	if mediaType != string(MediaTypeMovie) && mediaType != string(MediaTypeSeries) {
		return fmt.Errorf("unsupported discover media type")
	}
	if (item.TMDbID == "" || item.TMDbID == "0") && m.tmdbClient != nil {
		provider, externalID := "imdb", item.IMDbID
		if externalID == "" {
			provider, externalID = "tvdb", item.TVDbID
		}
		if externalID != "" {
			tmdbMedia := "movie"
			if mediaType == string(MediaTypeSeries) {
				tmdbMedia = "tv"
			}
			id, err := m.tmdbClient.ResolveExternalID(ctx, tmdbMedia, provider, externalID)
			if err != nil {
				return err
			}
			item.TMDbID = strconv.Itoa(id)
		}
	}
	seriesIDs := map[string]bool{}
	if mediaType == string(MediaTypeSeries) && m.seriesSvc != nil {
		all, err := m.seriesSvc.ListSeries(ctx)
		if err != nil {
			return err
		}
		for _, s := range all {
			if s.TMDBID != nil {
				seriesIDs[*s.TMDBID] = true
			}
		}
	}
	if !m.itemInLibrary(ctx, mediaType, item, seriesIDs) {
		return fmt.Errorf("title is not in the library")
	}
	item.Status = ItemStatusAdded
	return m.store.UpsertItem(ctx, item)
}

// itemInLibrary reports whether the given item is already present in the library.
func (m *SyncManager) itemInLibrary(ctx context.Context, mediaType string, item *ImportListItem, seriesTMDB map[string]bool) bool {
	if mediaType == string(MediaTypeSeries) {
		id := strings.TrimSpace(item.TMDbID)
		return id != "" && seriesTMDB[id]
	}
	if m.moviesSvc == nil {
		return false
	}
	if item.TMDbID != "" && item.TMDbID != "0" {
		if existing, err := m.moviesSvc.GetMovieByTMDBID(ctx, item.TMDbID); err == nil && existing != nil {
			return true
		}
	}
	if item.IMDbID != "" {
		if existing, err := m.moviesSvc.GetMovieByIMDBID(ctx, item.IMDbID); err == nil && existing != nil {
			return true
		}
	}
	return false
}

// AddDiscoverItem adds a single discover item to the library using its parent
// list's configuration (quality profile, library, search-on-add).
func (m *SyncManager) AddDiscoverItem(ctx context.Context, itemID string) (*ImportListItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, err := m.store.GetItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("discover item not found")
	}

	l, err := m.store.Get(ctx, item.ListID)
	if err != nil {
		return nil, err
	}
	if l == nil {
		return nil, fmt.Errorf("parent list not found")
	}

	mediaType := item.MediaType
	if mediaType == "" {
		mediaType = string(l.MediaType)
	}

	if mediaType == string(MediaTypeSeries) {
		err = m.addSeriesToLibrary(ctx, l, item)
	} else {
		err = m.addMovieToLibrary(ctx, l, item)
	}
	if err != nil {
		return nil, err
	}

	item.Status = ItemStatusAdded
	if uErr := m.store.UpsertItem(ctx, item); uErr != nil {
		m.logger.Error("import-lists: discover add status update failed", "error", uErr)
	}
	return item, nil
}

// dedupeKey returns a stable key for deduplicating items across lists.
func dedupeKey(item *ImportListItem) string {
	if id := strings.TrimSpace(item.TMDbID); id != "" && id != "0" {
		return "tmdb:" + id
	}
	if id := strings.TrimSpace(item.IMDbID); id != "" {
		return "imdb:" + id
	}
	year := 0
	if item.Year != nil {
		year = *item.Year
	}
	return fmt.Sprintf("title:%s:%d", strings.ToLower(strings.TrimSpace(item.Title)), year)
}
