package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ebenderooock/loom/internal/kernel/scheduler"
	"github.com/ebenderooock/loom/internal/rss"
	"github.com/ebenderooock/loom/internal/storage"
	dbpg "github.com/ebenderooock/loom/internal/storage/db/postgres"
	dbsqlite "github.com/ebenderooock/loom/internal/storage/db/sqlite"
)

// buildRSSManager initializes the RSS sync manager, loads enabled user sources,
// and registers a periodic job to synchronize feeds.
func buildRSSManager(ctx context.Context, sched *scheduler.Scheduler, db storage.DB, logger *slog.Logger) (*rss.SyncManager, error) {
	// Create storage and sync manager
	rssStorage := rss.NewStorage(db.DB())
	manager := rss.NewSyncManager(rssStorage, logger)

	// Load all enabled user sources from database and register them
	sourceSvc := rss.NewSourcesService(logger, db)
	enabledSources, err := listEnabledSources(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("load enabled sources: %w", err)
	}

	registered := 0
	for _, userSourceRow := range enabledSources {
		// Convert database row to UserSource using rss package's conversion logic
		userSource := convertDBUserSource(userSourceRow)
		if userSource == nil {
			continue
		}

		feedSource, err := sourceSvc.MakeFeedSource(userSource)
		if err != nil {
			logger.Warn("skip source: invalid config",
				"source_id", userSource.ID,
				"source_name", userSource.Name,
				"error", err,
			)
			continue
		}
		manager.RegisterSource(feedSource)
		registered++
		logger.Debug("registered feed source",
			"id", feedSource.ID(),
			"name", feedSource.Name(),
			"interval", feedSource.RefreshInterval(),
		)
	}
	logger.Info("RSS sources registered", "count", registered)

	// Register periodic sync job
	if err := registerRSSSync(ctx, sched, manager); err != nil {
		return nil, fmt.Errorf("register RSS sync job: %w", err)
	}

	return manager, nil
}

// registerRSSSync registers a periodic job to synchronize all RSS feeds.
// The job runs every 15 minutes.
const RSSyncJobName = "rss.sync"
const RSSyncSchedule = "*/15 * * * *" // Every 15 minutes

func registerRSSSync(ctx context.Context, sched *scheduler.Scheduler, manager *rss.SyncManager) error {
	if sched == nil || manager == nil {
		return fmt.Errorf("rss sync: scheduler and manager must not be nil")
	}

	handler := func(ctx context.Context) error {
		return manager.SyncFeeds(ctx)
	}

	return sched.Register(ctx, RSSyncJobName, RSSyncSchedule, handler, []byte(`{"builtin":true}`))
}

// listEnabledSources retrieves all enabled user sources using the appropriate database backend.
// Returns []interface{} to handle both sqlite and postgres UserSource types.
func listEnabledSources(ctx context.Context, db storage.DB) ([]interface{}, error) {
	sqlDB := db.DB()

	switch db.Engine() {
	case storage.EngineSQLite:
		q := dbsqlite.New(sqlDB)
		rows, err := q.ListEnabledUserSources(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]interface{}, len(rows))
		for i, r := range rows {
			result[i] = r
		}
		return result, nil

	case storage.EnginePostgres:
		q := dbpg.New(sqlDB)
		rows, err := q.ListEnabledUserSources(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]interface{}, len(rows))
		for i, r := range rows {
			result[i] = r
		}
		return result, nil

	default:
		return nil, fmt.Errorf("unsupported storage engine: %v", db.Engine())
	}
}

// convertDBUserSource converts a database UserSource row to rss.UserSource.
// Mirrors the rowToUserSource logic from sources_service.go.
func convertDBUserSource(row interface{}) *rss.UserSource {
	switch r := row.(type) {
	case dbsqlite.UserSource:
		us := &rss.UserSource{
			ID:      r.ID,
			Name:    r.Name,
			Type:    rss.SourceType(r.Type),
			Enabled: r.Enabled.Bool,
			Config:  []byte(r.Config),
		}
		if r.CreatedAt.Valid {
			us.CreatedAt = r.CreatedAt.Time.String()
		}
		if r.UpdatedAt.Valid {
			us.UpdatedAt = r.UpdatedAt.Time.String()
		}
		if r.LastSyncAt.Valid {
			lastSyncAt := r.LastSyncAt.Time.String()
			us.LastSyncAt = &lastSyncAt
		}
		return us

	case dbpg.UserSource:
		us := &rss.UserSource{
			ID:      r.ID,
			Name:    r.Name,
			Type:    rss.SourceType(r.Type),
			Enabled: r.Enabled.Bool,
			Config:  []byte(r.Config),
		}
		if r.CreatedAt.Valid {
			us.CreatedAt = r.CreatedAt.Time.String()
		}
		if r.UpdatedAt.Valid {
			us.UpdatedAt = r.UpdatedAt.Time.String()
		}
		if r.LastSyncAt.Valid {
			lastSyncAt := r.LastSyncAt.Time.String()
			us.LastSyncAt = &lastSyncAt
		}
		return us

	default:
		return nil
	}
}
