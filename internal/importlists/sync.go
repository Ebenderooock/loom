package importlists

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/connect"
	"github.com/ebenderooock/loom/internal/importlists/providers"
)

// SyncManager periodically syncs all enabled import lists.
type SyncManager struct {
	store      *Store
	connectSvc ConnectService
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

	// For Trakt lists, fill in credentials from the user's Trakt connection
	// if not explicitly set on the import list.
	if (l.ListType == ListTypeTraktList || l.ListType == ListTypeTraktWatchlist) && m.connectSvc != nil {
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

	return nil
}

func (m *SyncManager) providerFor(lt ListType) providers.ListProvider {
	switch lt {
	case ListTypeTraktList:
		return providers.NewTraktList()
	case ListTypeTraktWatchlist:
		return providers.NewTraktWatchlist()
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
