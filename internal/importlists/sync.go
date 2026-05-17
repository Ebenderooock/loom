package importlists

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/connect"
	"github.com/ebenderooock/loom/internal/importlists/providers"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

// SyncManager periodically syncs all enabled import lists.
type SyncManager struct {
	store      *Store
	connectSvc ConnectService
	moviesSvc  movies.Service
	seriesSvc  series.Service
	tmdbAPIKey string
	logger     *slog.Logger
	mu         sync.Mutex
	cancel     context.CancelFunc
	stopped    chan struct{}
}

// ConnectService provides access to configured connections (e.g. Trakt OAuth).
type ConnectService interface {
	ListConnections(ctx context.Context) ([]*connect.Connection, error)
}

// NewSyncManager creates a SyncManager.
func NewSyncManager(store *Store, logger *slog.Logger) *SyncManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &SyncManager{store: store, logger: logger}
}

// SetConnectService sets the connect service for credential lookups.
// This allows lazy injection when the connect service is created after
// the SyncManager.
func (m *SyncManager) SetConnectService(svc ConnectService) {
	m.connectSvc = svc
}

// SetMoviesService sets the movies service for adding movies from import lists.
func (m *SyncManager) SetMoviesService(svc movies.Service) {
	m.moviesSvc = svc
}

// SetSeriesService sets the series service for adding shows from import lists.
func (m *SyncManager) SetSeriesService(svc series.Service) {
	m.seriesSvc = svc
}

// SetTMDBAPIKey sets the TMDB API key for TMDb list providers.
func (m *SyncManager) SetTMDBAPIKey(key string) {
	m.tmdbAPIKey = key
}

// Start begins the background sync loop. It checks every minute for lists
// whose sync interval has elapsed.
func (m *SyncManager) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	m.stopped = make(chan struct{})

	go func() {
		defer close(m.stopped)
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		// Run an initial check immediately.
		m.tick(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.tick(ctx)
			}
		}
	}()
}

// Stop cancels the background loop and waits for it to exit.
func (m *SyncManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.stopped != nil {
		<-m.stopped
	}
}

func (m *SyncManager) tick(ctx context.Context) {
	lists, err := m.store.ListAll(ctx)
	if err != nil {
		m.logger.Error("import-lists: failed to list", "err", err)
		return
	}
	now := time.Now().UTC()
	for _, l := range lists {
		if !l.Enabled {
			continue
		}
		if l.LastSync != nil {
			nextSync := l.LastSync.Add(time.Duration(l.SyncIntervalMinutes) * time.Minute)
			if now.Before(nextSync) {
				continue
			}
		}
		if err := m.SyncList(ctx, l); err != nil {
			m.logger.Error("import-lists: sync failed",
				"list", l.Name, "id", l.ID, "err", err)
		}
	}
}

// SyncList fetches items from the list's provider and upserts them.
func (m *SyncManager) SyncList(ctx context.Context, l *ImportList) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("import-lists: syncing", "list", l.Name, "type", l.ListType)

	provider := m.providerFor(l.ListType)
	if provider == nil {
		m.logger.Warn("import-lists: unsupported type", "type", l.ListType)
		return nil
	}

	cfg := providers.ProviderConfig{
		URL:         l.URL,
		APIKey:      l.APIKey,
		AccessToken: l.AccessToken,
		Settings:    l.Settings,
	}

	// For Trakt lists, fill in credentials from the user's Trakt connection.
	if isTraktListType(l.ListType) && m.connectSvc != nil {
		if cfg.APIKey == "" || cfg.AccessToken == "" {
			if traktConn := m.findTraktConnection(ctx); traktConn != nil {
				if cfg.APIKey == "" {
					cfg.APIKey = traktConn.Settings.ClientID
				}
				if cfg.AccessToken == "" {
					cfg.AccessToken = traktConn.Settings.AccessToken
				}
			}
		}
	}

	// For TMDb lists, auto-fill the API key from the bundled key.
	if (l.ListType == ListTypeTMDbList || l.ListType == ListTypeTMDbPopular) && cfg.APIKey == "" {
		cfg.APIKey = m.tmdbAPIKey
	}

	fetched, err := provider.Fetch(ctx, cfg)
	if err != nil {
		return err
	}

	m.logger.Info("import-lists: fetched items", "list", l.Name, "count", len(fetched))

	for _, fi := range fetched {
		// Check exclusions
		excluded, err := m.store.IsExcluded(ctx, fi.IMDbID, fi.TMDbID, fi.TVDbID)
		if err != nil {
			m.logger.Error("import-lists: exclusion check failed", "err", err)
			continue
		}

		existing, err := m.store.FindItemByExternalID(ctx, l.ID, fi.ExternalID)
		if err != nil {
			m.logger.Error("import-lists: find item failed", "err", err)
			continue
		}

		status := ItemStatusPending
		if excluded {
			status = ItemStatusExcluded
		}

		if existing != nil {
			existing.Title = fi.Title
			existing.IMDbID = fi.IMDbID
			existing.TMDbID = fi.TMDbID
			existing.TVDbID = fi.TVDbID
			existing.MediaType = fi.MediaType
			if fi.Year != 0 {
				existing.Year = &fi.Year
			}
			if excluded {
				existing.Status = ItemStatusExcluded
			}
			if err := m.store.UpsertItem(ctx, existing); err != nil {
				m.logger.Error("import-lists: upsert existing failed", "err", err)
			}
			continue
		}

		year := fi.Year
		var yearPtr *int
		if year != 0 {
			yearPtr = &year
		}
		item := &ImportListItem{
			ListID:     l.ID,
			ExternalID: fi.ExternalID,
			Title:      fi.Title,
			Year:       yearPtr,
			IMDbID:     fi.IMDbID,
			TMDbID:     fi.TMDbID,
			TVDbID:     fi.TVDbID,
			MediaType:  fi.MediaType,
			Status:     status,
		}
		if err := m.store.UpsertItem(ctx, item); err != nil {
			m.logger.Error("import-lists: insert failed", "err", err)
		}
	}

	now := time.Now().UTC()
	if err := m.store.UpdateLastSync(ctx, l.ID, now); err != nil {
		m.logger.Error("import-lists: update last_sync failed", "err", err)
	}

	// Process pending items — actually add them to the library.
	m.processPendingItems(ctx, l)

	return nil
}

func (m *SyncManager) providerFor(lt ListType) providers.ListProvider {
	switch lt {
	case ListTypeTraktList:
		return providers.NewTraktList()
	case ListTypeTraktWatchlist:
		return providers.NewTraktWatchlist()
	case ListTypeTraktPopular:
		return providers.NewTraktPopular()
	case ListTypeTraktTrending:
		return providers.NewTraktTrending()
	case ListTypeTraktAnticipated:
		return providers.NewTraktAnticipated()
	case ListTypeIMDbList:
		return providers.NewIMDbList()
	case ListTypeIMDbWatchlist:
		return providers.NewIMDbWatchlist()
	case ListTypeTMDbList:
		return providers.NewTMDbList()
	case ListTypeTMDbPopular:
		return providers.NewTMDbPopular()
	case ListTypePlexWatchlist:
		return providers.NewPlexWatchlist()
	case ListTypeRSS:
		return providers.NewRSS()
	default:
		return nil
	}
}

// isTraktListType returns true for any Trakt-backed list type.
func isTraktListType(lt ListType) bool {
	switch lt {
	case ListTypeTraktList, ListTypeTraktWatchlist,
		ListTypeTraktPopular, ListTypeTraktTrending, ListTypeTraktAnticipated:
		return true
	default:
		return false
	}
}

// GetTraktUserLists fetches the authenticated user's custom Trakt lists
// using the stored Trakt connection credentials.
func (m *SyncManager) GetTraktUserLists(ctx context.Context) ([]providers.TraktUserList, error) {
	if m.connectSvc == nil {
		return nil, fmt.Errorf("connect service not configured")
	}
	conn := m.findTraktConnection(ctx)
	if conn == nil {
		return nil, fmt.Errorf("no Trakt connection found; connect Trakt in Settings → Connections")
	}
	return providers.FetchTraktUserLists(ctx, conn.Settings.ClientID, conn.Settings.AccessToken)
}

// findTraktConnection returns the first enabled Trakt connection, or nil.
func (m *SyncManager) findTraktConnection(ctx context.Context) *connect.Connection {
	conns, err := m.connectSvc.ListConnections(ctx)
	if err != nil {
		m.logger.Warn("import-lists: failed to list connections for Trakt credentials", "err", err)
		return nil
	}
	for _, c := range conns {
		if c.Provider == connect.ProviderTrakt && c.Enabled && c.Settings.AccessToken != "" {
			return c
		}
	}
	return nil
}

// processPendingItems adds pending import list items to the library.
func (m *SyncManager) processPendingItems(ctx context.Context, l *ImportList) {
	items, err := m.store.ListItems(ctx, l.ID)
	if err != nil {
		m.logger.Error("import-lists: list pending items failed", "err", err)
		return
	}

	for _, item := range items {
		if item.Status != ItemStatusPending {
			continue
		}

		// Determine effective media type: item-level overrides list-level.
		mediaType := item.MediaType
		if mediaType == "" {
			mediaType = string(l.MediaType)
		}

		var addErr error
		if mediaType == string(MediaTypeSeries) {
			addErr = m.addSeriesToLibrary(ctx, l, item)
		} else {
			addErr = m.addMovieToLibrary(ctx, l, item)
		}

		if addErr != nil {
			m.logger.Error("import-lists: add item failed",
				"title", item.Title, "err", addErr)
			item.Status = ItemStatusFailed
		} else {
			item.Status = ItemStatusAdded
		}

		if err := m.store.UpsertItem(ctx, item); err != nil {
			m.logger.Error("import-lists: update item status failed", "err", err)
		}
	}
}

// addMovieToLibrary adds a movie to the library via the movies service.
func (m *SyncManager) addMovieToLibrary(ctx context.Context, l *ImportList, item *ImportListItem) error {
	if m.moviesSvc == nil {
		return fmt.Errorf("movies service not configured")
	}

	// Check if the movie already exists in the library by TMDb ID.
	if item.TMDbID != "" && item.TMDbID != "0" {
		existing, err := m.findExistingMovieByTMDBID(ctx, item.TMDbID)
		if err == nil && existing != nil {
			return nil // already in library
		}
	}

	year := 0
	if item.Year != nil {
		year = *item.Year
	}

	tmdbID := item.TMDbID
	imdbID := item.IMDbID

	movie := &movies.Movie{
		ID:               makeMovieSlug(item.Title, year),
		Title:            item.Title,
		Year:             year,
		MonitoringStatus: movies.MonitoringStatusMonitored,
		Status:           movies.MovieStatusMissing,
		QualityProfileID: l.QualityProfileID,
		LibraryID:        l.LibraryPath,
	}
	if tmdbID != "" && tmdbID != "0" {
		movie.TMDBID = &tmdbID
	}
	if imdbID != "" {
		movie.IMDBID = &imdbID
	}

	if err := m.moviesSvc.AddMovie(ctx, movie); err != nil {
		// If the movie already exists (duplicate slug), treat as success.
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return fmt.Errorf("add movie %q: %w", item.Title, err)
	}

	m.logger.Info("import-lists: added movie", "title", item.Title, "tmdb_id", tmdbID)
	return nil
}

// addSeriesToLibrary adds a series to the library via the series service.
func (m *SyncManager) addSeriesToLibrary(ctx context.Context, l *ImportList, item *ImportListItem) error {
	if m.seriesSvc == nil {
		return fmt.Errorf("series service not configured")
	}

	// Check if the series already exists by TMDb ID.
	if item.TMDbID != "" && item.TMDbID != "0" {
		existing, err := m.findExistingSeriesByTMDBID(ctx, item.TMDbID)
		if err == nil && existing != nil {
			return nil // already in library
		}
	}

	req := &series.AddSeriesRequest{
		TMDBID:           item.TMDbID,
		QualityProfileID: l.QualityProfileID,
		LibraryID:        l.LibraryPath,
		Search:           l.SearchOnAdd,
	}

	if _, err := m.seriesSvc.AddSeries(ctx, req); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return fmt.Errorf("add series %q: %w", item.Title, err)
	}

	m.logger.Info("import-lists: added series", "title", item.Title, "tmdb_id", item.TMDbID)
	return nil
}

// findExistingMovieByTMDBID checks whether a movie with the given TMDB ID
// already exists in the library.
func (m *SyncManager) findExistingMovieByTMDBID(ctx context.Context, tmdbID string) (*movies.Movie, error) {
	allMovies, err := m.moviesSvc.ListMovies(ctx, 0, 0)
	if err != nil {
		return nil, err
	}
	for _, mv := range allMovies {
		if mv.TMDBID != nil && *mv.TMDBID == tmdbID {
			return mv, nil
		}
	}
	return nil, nil
}

// findExistingSeriesByTMDBID checks whether a series with the given TMDB ID
// already exists in the library.
func (m *SyncManager) findExistingSeriesByTMDBID(ctx context.Context, tmdbID string) (*series.Series, error) {
	allSeries, err := m.seriesSvc.ListSeries(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range allSeries {
		if s.TMDBID != nil && *s.TMDBID == tmdbID {
			return s, nil
		}
	}
	return nil, nil
}

// makeMovieSlug generates a URL-safe slug for a movie.
func makeMovieSlug(title string, year int) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		s = "untitled"
	}
	if year > 0 {
		s = s + "-" + strconv.Itoa(year)
	}
	return s
}
