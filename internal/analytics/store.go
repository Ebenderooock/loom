package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const tsLayout = time.RFC3339

// Store persists play history using raw SQL.
type Store struct {
	db *sql.DB
}

// NewStore creates a play-history store.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func parseTS(s string) time.Time {
	t, _ := time.Parse(tsLayout, s)
	return t
}

// OpenRowsForConn returns all open (not-yet-ended) history rows for a
// connection.
func (s *Store) OpenRowsForConn(ctx context.Context, connID string) ([]HistoryRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, connection_id, provider, session_key, media_id, user, media_type,
		       title, grandparent_title, full_title, device, transcode,
		       started_at, last_seen_at, last_position_ms, duration_ms, watched_ms, bitrate_kbps
		FROM play_history
		WHERE connection_id = ? AND ended_at IS NULL`, connID)
	if err != nil {
		return nil, fmt.Errorf("open rows: %w", err)
	}
	defer rows.Close()

	var out []HistoryRecord
	for rows.Next() {
		var r HistoryRecord
		var started, lastSeen string
		var transcode int
		if err := rows.Scan(&r.ID, &r.ConnectionID, &r.Provider, &r.SessionKey, &r.MediaID,
			&r.User, &r.MediaType, &r.Title, &r.GrandparentTitle, &r.FullTitle, &r.Device, &transcode,
			&started, &lastSeen, &r.LastPositionMs, &r.DurationMs, &r.WatchedMs, &r.BitrateKbps); err != nil {
			return nil, fmt.Errorf("scan open row: %w", err)
		}
		r.Transcode = transcode != 0
		r.StartedAt = parseTS(started)
		r.LastSeenAt = parseTS(lastSeen)
		out = append(out, r)
	}
	return out, rows.Err()
}

// InsertOpen inserts a new open playback row.
func (s *Store) InsertOpen(ctx context.Context, r HistoryRecord) error {
	transcode := 0
	if r.Transcode {
		transcode = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO play_history
		(id, connection_id, provider, session_key, media_id, user, media_type,
		 title, grandparent_title, full_title, device, transcode,
		 started_at, last_seen_at, last_position_ms, duration_ms, watched_ms, bitrate_kbps)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.ConnectionID, r.Provider, r.SessionKey, r.MediaID, r.User, r.MediaType,
		r.Title, r.GrandparentTitle, r.FullTitle, r.Device, transcode,
		r.StartedAt.UTC().Format(tsLayout), r.LastSeenAt.UTC().Format(tsLayout),
		r.LastPositionMs, r.DurationMs, r.WatchedMs, r.BitrateKbps)
	if err != nil {
		return fmt.Errorf("insert open: %w", err)
	}
	return nil
}

// UpdateOpen advances an open row with the latest sample. The transcode flag is
// sticky: once a session is observed transcoding it stays marked, so a session
// that switches modes still counts as a transcode play. Bitrate is recorded
// only when the sample reports a non-zero value (direct play often reports 0).
func (s *Store) UpdateOpen(ctx context.Context, id string, lastSeen time.Time, positionMs, watchedMs int64, transcode bool, bitrateKbps int64) error {
	t := 0
	if transcode {
		t = 1
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE play_history
		SET last_seen_at = ?, last_position_ms = ?, watched_ms = ?,
		    transcode = CASE WHEN ? = 1 THEN 1 ELSE transcode END,
		    bitrate_kbps = CASE WHEN ? > 0 THEN ? ELSE bitrate_kbps END
		WHERE id = ?`,
		lastSeen.UTC().Format(tsLayout), positionMs, watchedMs, t, bitrateKbps, bitrateKbps, id)
	if err != nil {
		return fmt.Errorf("update open: %w", err)
	}
	return nil
}

// Close finalises an open row, setting ended_at and a reason. It returns true
// when a row was actually closed (so callers only emit a stop event for genuine
// transitions, never for a no-op close).
func (s *Store) Close(ctx context.Context, id string, endedAt time.Time, reason string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE play_history SET ended_at = ?, end_reason = ? WHERE id = ? AND ended_at IS NULL`,
		endedAt.UTC().Format(tsLayout), reason, id)
	if err != nil {
		return false, fmt.Errorf("close row: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CloseAllOpen finalises every open row (used on startup to clear orphans).
// Each row is ended at its own last_seen_at.
func (s *Store) CloseAllOpen(ctx context.Context, reason string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE play_history SET ended_at = last_seen_at, end_reason = ?
		WHERE ended_at IS NULL`, reason)
	if err != nil {
		return 0, fmt.Errorf("close all open: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ListHistory returns finished and in-progress rows, most recent first.
func (s *Store) ListHistory(ctx context.Context, f HistoryFilter) ([]HistoryRecord, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT id, connection_id, provider, session_key, media_id, user, media_type,
		       title, grandparent_title, full_title, device, transcode,
		       started_at, last_seen_at, last_position_ms, duration_ms, watched_ms, bitrate_kbps, ended_at
		FROM play_history`
	args := []any{}
	if f.User != "" {
		query += ` WHERE user = ?`
		args = append(args, f.User)
	}
	query += ` ORDER BY started_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, f.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	defer rows.Close()

	out := []HistoryRecord{}
	for rows.Next() {
		var r HistoryRecord
		var started, lastSeen string
		var ended sql.NullString
		var transcode int
		if err := rows.Scan(&r.ID, &r.ConnectionID, &r.Provider, &r.SessionKey, &r.MediaID,
			&r.User, &r.MediaType, &r.Title, &r.GrandparentTitle, &r.FullTitle, &r.Device, &transcode,
			&started, &lastSeen, &r.LastPositionMs, &r.DurationMs, &r.WatchedMs, &r.BitrateKbps, &ended); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		r.Transcode = transcode != 0
		r.StartedAt = parseTS(started)
		r.LastSeenAt = parseTS(lastSeen)
		if ended.Valid && ended.String != "" {
			t := parseTS(ended.String)
			r.EndedAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Stats computes the analytics report over the given window. Only plays with at
// least minWatchedMs of observed playback count toward the aggregates, to keep
// brief previews/accidental starts out of the reports.
func (s *Store) Stats(ctx context.Context, since time.Time, windowDays int, minWatchedMs int64) (*Stats, error) {
	sinceStr := since.UTC().Format(tsLayout)
	st := &Stats{WindowDays: windowDays, TopUsers: []UserStat{}, TopMedia: []MediaStat{}, LeastMedia: []MediaStat{}, PlaysPerDay: []DayCount{}}

	// Totals.
	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT user), COALESCE(SUM(watched_ms),0),
		       COALESCE(SUM(CASE WHEN transcode = 1 THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN transcode = 0 THEN 1 ELSE 0 END),0),
		       COALESCE((SELECT CAST(AVG(bitrate_kbps) AS INTEGER) FROM play_history
		                 WHERE started_at >= ? AND watched_ms >= ? AND bitrate_kbps > 0),0)
		FROM play_history WHERE started_at >= ? AND watched_ms >= ?`,
		sinceStr, minWatchedMs, sinceStr, minWatchedMs)
	if err := row.Scan(&st.Totals.Plays, &st.Totals.UniqueUsers, &st.Totals.WatchedMs,
		&st.Totals.TranscodePlays, &st.Totals.DirectPlays, &st.Totals.AvgBitrateKbps); err != nil {
		return nil, fmt.Errorf("stats totals: %w", err)
	}

	// Top users.
	uRows, err := s.db.QueryContext(ctx, `
		SELECT user, COUNT(*) plays, COALESCE(SUM(watched_ms),0)
		FROM play_history WHERE started_at >= ? AND watched_ms >= ? AND user <> ''
		GROUP BY user ORDER BY plays DESC, watched_ms DESC LIMIT 10`, sinceStr, minWatchedMs)
	if err != nil {
		return nil, fmt.Errorf("stats users: %w", err)
	}
	defer uRows.Close()
	for uRows.Next() {
		var u UserStat
		if err := uRows.Scan(&u.User, &u.Plays, &u.WatchedMs); err != nil {
			return nil, fmt.Errorf("scan user stat: %w", err)
		}
		st.TopUsers = append(st.TopUsers, u)
	}
	if err := uRows.Err(); err != nil {
		return nil, err
	}

	// Media aggregate (grouped by stable media id, falling back to title).
	mediaQuery := func(order string) ([]MediaStat, error) {
		rows, err := s.db.QueryContext(ctx, `
			SELECT COALESCE(NULLIF(media_id,''), full_title) AS key,
			       MAX(full_title), MAX(media_type), COUNT(*) plays, COALESCE(SUM(watched_ms),0)
			FROM play_history WHERE started_at >= ? AND watched_ms >= ?
			GROUP BY key ORDER BY plays `+order+`, watched_ms `+order+` LIMIT 10`, sinceStr, minWatchedMs)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []MediaStat{}
		for rows.Next() {
			var m MediaStat
			if err := rows.Scan(&m.MediaID, &m.Title, &m.MediaType, &m.Plays, &m.WatchedMs); err != nil {
				return nil, err
			}
			out = append(out, m)
		}
		return out, rows.Err()
	}
	if st.TopMedia, err = mediaQuery("DESC"); err != nil {
		return nil, fmt.Errorf("stats top media: %w", err)
	}
	if st.LeastMedia, err = mediaQuery("ASC"); err != nil {
		return nil, fmt.Errorf("stats least media: %w", err)
	}

	// Plays per day.
	dRows, err := s.db.QueryContext(ctx, `
		SELECT date(started_at) d, COUNT(*)
		FROM play_history WHERE started_at >= ? AND watched_ms >= ?
		GROUP BY d ORDER BY d ASC`, sinceStr, minWatchedMs)
	if err != nil {
		return nil, fmt.Errorf("stats days: %w", err)
	}
	defer dRows.Close()
	for dRows.Next() {
		var d DayCount
		if err := dRows.Scan(&d.Day, &d.Plays); err != nil {
			return nil, fmt.Errorf("scan day: %w", err)
		}
		st.PlaysPerDay = append(st.PlaysPerDay, d)
	}
	return st, dRows.Err()
}
