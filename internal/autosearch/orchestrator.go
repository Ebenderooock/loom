package autosearch

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/rss"
)

// SearchEvent represents a triggered search for a matched item.
type SearchEvent struct {
	ID        string
	RSSItemID string
	MovieID   string
	Title     string
	Year      int
	CreatedAt time.Time
	Status    string // pending, searching, found, not_found, error
	ErrorMsg  string
}

// Orchestrator coordinates RSS monitoring with automatic searches for matched items.
type Orchestrator struct {
	db       *sql.DB
	sync     *rss.SyncManager
	matcher  *Matcher
	mu       sync.RWMutex
	searches map[string]*SearchEvent // ItemID -> SearchEvent
}

// NewOrchestrator creates a new auto-search orchestrator.
func NewOrchestrator(db *sql.DB, sync *rss.SyncManager, matcher *Matcher) *Orchestrator {
	return &Orchestrator{
		db:       db,
		sync:     sync,
		matcher:  matcher,
		searches: make(map[string]*SearchEvent),
	}
}

// ScanAndMatch processes all recent RSS items, matching them against library wants.
// Returns a list of potential matches.
func (o *Orchestrator) ScanAndMatch(ctx context.Context) ([]Match, error) {
	items, err := o.sync.GetRecentItems(ctx, 100) // Scan recent 100 items
	if err != nil {
		return nil, fmt.Errorf("failed to get recent items: %w", err)
	}

	var allMatches []Match
	for _, item := range items {
		matches := o.matcher.FindMatches(item)
		allMatches = append(allMatches, matches...)
	}

	return allMatches, nil
}

// TriggerSearches creates search events for matched items and persists them.
// Returns the number of searches triggered.
func (o *Orchestrator) TriggerSearches(ctx context.Context, matches []Match) (int, error) {
	if len(matches) == 0 {
		return 0, nil
	}

	// Create search_history table if it doesn't exist
	if err := o.ensureSchema(); err != nil {
		return 0, fmt.Errorf("failed to ensure schema: %w", err)
	}

	count := 0
	for _, match := range matches {
		event := &SearchEvent{
			ID:        fmt.Sprintf("%s_%s", match.ItemID, match.MovieID),
			RSSItemID: match.ItemID,
			MovieID:   match.MovieID,
			Title:     match.Title,
			Year:      match.Year,
			CreatedAt: time.Now(),
			Status:    "pending",
		}

		if err := o.storeSearchEvent(ctx, event); err != nil {
			// Log error but continue with other matches
			slog.Warn("failed to store search event", "error", err, "item_id", match.ItemID)
			continue
		}

		o.mu.Lock()
		o.searches[match.ItemID] = event
		o.mu.Unlock()

		count++
	}

	return count, nil
}

// GetSearchHistory retrieves search events for a specific movie.
func (o *Orchestrator) GetSearchHistory(ctx context.Context, movieID string, limit int) ([]SearchEvent, error) {
	query := `
		SELECT id, rss_item_id, movie_id, title, year, created_at, status, error_msg
		FROM search_history
		WHERE movie_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := o.db.QueryContext(ctx, query, movieID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []SearchEvent
	for rows.Next() {
		var event SearchEvent
		if err := rows.Scan(
			&event.ID, &event.RSSItemID, &event.MovieID, &event.Title,
			&event.Year, &event.CreatedAt, &event.Status, &event.ErrorMsg,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

// UpdateSearchStatus updates the status of a search event.
func (o *Orchestrator) UpdateSearchStatus(ctx context.Context, eventID, status, errorMsg string) error {
	query := `
		UPDATE search_history
		SET status = ?, error_msg = ?
		WHERE id = ?
	`

	_, err := o.db.ExecContext(ctx, query, status, errorMsg, eventID)
	return err
}

// storeSearchEvent persists a search event to the database.
func (o *Orchestrator) storeSearchEvent(ctx context.Context, event *SearchEvent) error {
	query := `
		INSERT INTO search_history (id, rss_item_id, movie_id, title, year, created_at, status, error_msg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := o.db.ExecContext(ctx, query,
		event.ID, event.RSSItemID, event.MovieID, event.Title, event.Year, event.CreatedAt, event.Status, event.ErrorMsg,
	)
	return err
}

// ensureSchema creates the search_history table if it doesn't exist.
func (o *Orchestrator) ensureSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS search_history (
			id TEXT PRIMARY KEY,
			rss_item_id TEXT NOT NULL,
			movie_id TEXT NOT NULL,
			title TEXT NOT NULL,
			year INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			status TEXT DEFAULT 'pending',
			error_msg TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_search_history_movie_id ON search_history(movie_id);
		CREATE INDEX IF NOT EXISTS idx_search_history_created_at ON search_history(created_at);
	`

	_, err := o.db.Exec(schema)
	return err
}

// Stats returns statistics about search operations.
type OrchestrationStats struct {
	TotalMatches      int
	PendingSearches   int
	CompletedSearches int
	FailedSearches    int
}

// GetStats returns current orchestration statistics.
func (o *Orchestrator) GetStats(ctx context.Context) (OrchestrationStats, error) {
	stats := OrchestrationStats{}

	// Count pending
	err := o.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM search_history WHERE status = 'pending'",
	).Scan(&stats.PendingSearches)
	if err != nil && err != sql.ErrNoRows {
		return stats, err
	}

	// Count completed
	err = o.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM search_history WHERE status IN ('found', 'not_found')",
	).Scan(&stats.CompletedSearches)
	if err != nil && err != sql.ErrNoRows {
		return stats, err
	}

	// Count failed
	err = o.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM search_history WHERE status = 'error'",
	).Scan(&stats.FailedSearches)
	if err != nil && err != sql.ErrNoRows {
		return stats, err
	}

	stats.TotalMatches = stats.PendingSearches + stats.CompletedSearches + stats.FailedSearches
	return stats, nil
}
