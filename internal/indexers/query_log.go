package indexers

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// QueryLogEntry is a single logged search query with optional indexer breakdown.
type QueryLogEntry struct {
	ID           string              `json:"id"`
	Query        string              `json:"query"`
	QueryType    string              `json:"query_type"`
	MediaType    string              `json:"media_type"`
	MediaID      string              `json:"media_id"`
	StartedAt    time.Time           `json:"started_at"`
	FinishedAt   *time.Time          `json:"finished_at,omitempty"`
	TotalResults int                 `json:"total_results"`
	Status       string              `json:"status"`
	Indexers     []IndexerQueryEntry `json:"indexers,omitempty"`
}

// IndexerQueryEntry records the contribution of a single indexer to a query.
type IndexerQueryEntry struct {
	ID          string     `json:"id"`
	IndexerID   string     `json:"indexer_id"`
	IndexerName string     `json:"indexer_name"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	LatencyMs   int64      `json:"latency_ms"`
	ResultCount int        `json:"result_count"`
	Error       string     `json:"error,omitempty"`
	Status      string     `json:"status"`
}

// QueryLog persists per-query search diagnostics.
type QueryLog struct {
	db *sql.DB
}

// NewQueryLog creates a QueryLog backed by db.
func NewQueryLog(db *sql.DB) *QueryLog {
	return &QueryLog{db: db}
}

// NewQueryID returns a new UUID suitable for query log entries.
func NewQueryID() string {
	return uuid.New().String()
}

// StartQuery inserts a new running query.
func (q *QueryLog) StartQuery(ctx context.Context, id, query, queryType, mediaType, mediaID string) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO search_query_log (id, query, query_type, media_type, media_id, started_at, status)
		 VALUES (?, ?, ?, ?, ?, ?, 'running')`,
		id, query, queryType, mediaType, mediaID, time.Now().UTC(),
	)
	return err
}

// StartIndexerQuery inserts a running indexer-level entry.
func (q *QueryLog) StartIndexerQuery(ctx context.Context, id, queryID, indexerID, indexerName string) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO search_query_indexer_log (id, query_id, indexer_id, indexer_name, started_at, status)
		 VALUES (?, ?, ?, ?, ?, 'running')`,
		id, queryID, indexerID, indexerName, time.Now().UTC(),
	)
	return err
}

// FinishIndexerQuery marks an indexer query as completed or failed.
func (q *QueryLog) FinishIndexerQuery(ctx context.Context, id string, resultCount int, searchErr error) error {
	now := time.Now().UTC()
	status := "completed"
	errMsg := ""
	if searchErr != nil {
		status = "failed"
		errMsg = searchErr.Error()
	}
	_, err := q.db.ExecContext(ctx,
		`UPDATE search_query_indexer_log
		 SET finished_at = ?, latency_ms = CAST((julianday(?) - julianday(started_at)) * 86400000 AS INTEGER),
		     result_count = ?, error = ?, status = ?
		 WHERE id = ?`,
		now, now, resultCount, errMsg, status, id,
	)
	return err
}

// FinishQuery marks a top-level query as completed or failed.
func (q *QueryLog) FinishQuery(ctx context.Context, id string, totalResults int, searchErr error) error {
	now := time.Now().UTC()
	status := "completed"
	if searchErr != nil {
		status = "failed"
	}
	_, err := q.db.ExecContext(ctx,
		`UPDATE search_query_log SET finished_at = ?, total_results = ?, status = ? WHERE id = ?`,
		now, totalResults, status, id,
	)
	return err
}

// ListQueries returns recent queries with pagination, newest first.
func (q *QueryLog) ListQueries(ctx context.Context, limit, offset int) ([]QueryLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := q.db.QueryContext(ctx,
		`SELECT id, query, query_type, media_type, media_id, started_at, finished_at, total_results, status
		 FROM search_query_log ORDER BY started_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QueryLogEntry
	for rows.Next() {
		var e QueryLogEntry
		var finAt sql.NullTime
		if err := rows.Scan(&e.ID, &e.Query, &e.QueryType, &e.MediaType, &e.MediaID,
			&e.StartedAt, &finAt, &e.TotalResults, &e.Status); err != nil {
			return nil, err
		}
		if finAt.Valid {
			e.FinishedAt = &finAt.Time
		}
		out = append(out, e)
	}
	if out == nil {
		out = []QueryLogEntry{}
	}
	return out, rows.Err()
}

// GetQuery returns a single query with its indexer breakdown.
func (q *QueryLog) GetQuery(ctx context.Context, id string) (*QueryLogEntry, error) {
	var e QueryLogEntry
	var finAt sql.NullTime
	err := q.db.QueryRowContext(ctx,
		`SELECT id, query, query_type, media_type, media_id, started_at, finished_at, total_results, status
		 FROM search_query_log WHERE id = ?`, id,
	).Scan(&e.ID, &e.Query, &e.QueryType, &e.MediaType, &e.MediaID,
		&e.StartedAt, &finAt, &e.TotalResults, &e.Status)
	if err != nil {
		return nil, err
	}
	if finAt.Valid {
		e.FinishedAt = &finAt.Time
	}

	rows, err := q.db.QueryContext(ctx,
		`SELECT id, indexer_id, indexer_name, started_at, finished_at, latency_ms, result_count, error, status
		 FROM search_query_indexer_log WHERE query_id = ? ORDER BY started_at`, id)
	if err != nil {
		return &e, err
	}
	defer rows.Close()

	for rows.Next() {
		var ie IndexerQueryEntry
		var iFinAt sql.NullTime
		if err := rows.Scan(&ie.ID, &ie.IndexerID, &ie.IndexerName,
			&ie.StartedAt, &iFinAt, &ie.LatencyMs, &ie.ResultCount, &ie.Error, &ie.Status); err != nil {
			return &e, err
		}
		if iFinAt.Valid {
			ie.FinishedAt = &iFinAt.Time
		}
		e.Indexers = append(e.Indexers, ie)
	}
	if e.Indexers == nil {
		e.Indexers = []IndexerQueryEntry{}
	}
	return &e, rows.Err()
}

// PruneOlderThan deletes query log entries older than age. Returns the
// number of top-level rows deleted (child rows cascade).
func (q *QueryLog) PruneOlderThan(ctx context.Context, age time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-age)
	res, err := q.db.ExecContext(ctx,
		`DELETE FROM search_query_log WHERE started_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
