package rss

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// SyncManager orchestrates periodic RSS feed synchronization from multiple sources.
type SyncManager struct {
	storage *Storage
	logger  *slog.Logger
	sources map[string]FeedSource
	mu      sync.RWMutex
	stats   Stats
}

// NewSyncManager creates a new RSS sync manager.
func NewSyncManager(storage *Storage, logger *slog.Logger) *SyncManager {
	return &SyncManager{
		storage: storage,
		logger:  logger,
		sources: make(map[string]FeedSource),
	}
}

// RegisterSource adds a new feed source to monitor.
func (m *SyncManager) RegisterSource(source FeedSource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sources[source.ID()] = source
	m.logger.Debug("registered RSS source", slog.String("id", source.ID()), slog.String("name", source.Name()))
}

// UnregisterSource stops monitoring a feed source.
func (m *SyncManager) UnregisterSource(sourceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sources, sourceID)
	m.logger.Debug("unregistered RSS source", slog.String("id", sourceID))
}

// SyncFeeds fetches items from all registered sources and stores them.
// Returns error only if a critical operation fails (not individual feed failures).
func (m *SyncManager) SyncFeeds(ctx context.Context) error {
	m.mu.RLock()
	sources := make([]FeedSource, 0, len(m.sources))
	for _, src := range m.sources {
		sources = append(sources, src)
	}
	m.mu.RUnlock()

	if len(sources) == 0 {
		m.logger.Debug("no RSS sources registered")
		return nil
	}

	m.logger.Info("starting RSS sync", slog.Int("sources", len(sources)))
	start := time.Now()

	var totalStored, totalDeduped int64
	for _, source := range sources {
		if err := m.syncSource(ctx, source, &totalStored, &totalDeduped); err != nil {
			// Log but continue syncing other sources
			m.logger.Error("failed to sync source", slog.String("source_id", source.ID()), slog.String("error", err.Error()))
			m.mu.Lock()
			m.stats.FailedSyncs++
			m.mu.Unlock()
		}
	}

	duration := time.Since(start)
	m.mu.Lock()
	m.stats.TotalSyncs++
	m.stats.SuccessfulSyncs++
	m.stats.LastSyncAt = time.Now().UTC()
	m.stats.ItemsStored += totalStored
	m.stats.ItemsDeduped += totalDeduped
	m.mu.Unlock()

	m.logger.Info("RSS sync completed",
		slog.Int64("stored", totalStored),
		slog.Int64("deduped", totalDeduped),
		slog.Duration("duration", duration),
	)

	return nil
}

// syncSource fetches and stores items from a single source.
func (m *SyncManager) syncSource(ctx context.Context, source FeedSource, totalStored, totalDeduped *int64) error {
	m.logger.Debug("syncing source", slog.String("source_id", source.ID()), slog.String("name", source.Name()))

	items, err := source.Fetch(ctx)
	if err != nil {
		return err
	}

	m.logger.Debug("fetched items", slog.String("source_id", source.ID()), slog.Int("count", len(items)))

	if len(items) == 0 {
		return nil
	}

	stored, deduped, err := m.storage.StoreItems(ctx, items)
	if err != nil {
		return err
	}

	*totalStored += int64(stored)
	*totalDeduped += int64(deduped)

	m.logger.Debug("stored items",
		slog.String("source_id", source.ID()),
		slog.Int("stored", stored),
		slog.Int("deduped", deduped),
	)

	return nil
}

// SyncSourceAsync syncs a specific source asynchronously, reporting progress via callback.
func (m *SyncManager) SyncSourceAsync(ctx context.Context, sourceID string, callback func(error)) {
	go func() {
		m.mu.RLock()
		source, ok := m.sources[sourceID]
		m.mu.RUnlock()

		if !ok {
			callback(ErrSourceNotFound)
			return
		}

		items, err := source.Fetch(ctx)
		if err != nil {
			callback(err)
			return
		}

		_, _, err = m.storage.StoreItems(ctx, items)
		callback(err)
	}()
}

// GetStats returns current sync statistics.
func (m *SyncManager) GetStats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

// GetRecentItems retrieves recent items from storage.
func (m *SyncManager) GetRecentItems(ctx context.Context, limit int) ([]*Item, error) {
	return m.storage.GetRecentItems(ctx, limit, "")
}

// GetSourceItems retrieves items from a specific source.
func (m *SyncManager) GetSourceItems(ctx context.Context, sourceID string, limit int) ([]*Item, error) {
	return m.storage.GetRecentItems(ctx, limit, sourceID)
}

// CleanupOldItems removes items older than the specified duration.
func (m *SyncManager) CleanupOldItems(ctx context.Context, olderThan time.Duration) error {
	deleted, err := m.storage.DeleteOldItems(ctx, olderThan)
	if err != nil {
		return err
	}
	m.logger.Info("cleaned up old RSS items", slog.Int64("deleted", deleted))
	return nil
}
