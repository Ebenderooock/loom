package importlists

import (
	"context"
	"fmt"
	"strings"
)

// DiscoverItem is an aggregated, library-annotated item shown in the Discover UI.
type DiscoverItem struct {
	*ImportListItem
	ListName  string `json:"list_name"`
	InLibrary bool   `json:"in_library"`
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
			m.logger.Warn("import-lists: discover list series failed", "err", err)
		}
	}

	seen := map[string]bool{}
	var out []*DiscoverItem
	for _, row := range rows {
		item := row.Item
		key := dedupeKey(item)
		if seen[key] {
			continue
		}
		seen[key] = true

		di := &DiscoverItem{ImportListItem: item, ListName: row.ListName}
		di.InLibrary = m.itemInLibrary(ctx, row.MediaType, item, seriesTMDB)
		out = append(out, di)
	}
	return out, nil
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
		m.logger.Error("import-lists: discover add status update failed", "err", uErr)
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
