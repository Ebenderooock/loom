package libraries

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type healthRecord struct {
	path, group, version string
	orphan               bool
}

// healthRecords limits each source before materializing it. library_files alone
// is not authoritative: imports can populate domain file tables independently.
func (s *Store) healthRecords(ctx context.Context, lib *Library, limit int) ([]healthRecord, bool, error) {
	root := filepath.Clean(lib.Path)
	prefix := root + string(filepath.Separator)
	if root == string(filepath.Separator) {
		prefix = root
	}
	queries := []struct {
		sql  string
		args []any
	}{
		{`SELECT path,
			CASE
				WHEN EXISTS (SELECT 1 FROM movies WHERE id = media_id AND deleted_at IS NULL)
					THEN 'movie:' || (SELECT COALESCE(NULLIF(tmdb_id, ''), id) FROM movies WHERE id = media_id)
				WHEN EXISTS (SELECT 1 FROM episodes WHERE id = media_id) THEN 'episode:' || media_id
				WHEN EXISTS (SELECT 1 FROM tracks WHERE id = media_id) THEN 'track:' || media_id
				ELSE '' END,
			'', CASE WHEN media_id IS NOT NULL AND media_id != ''
			AND NOT EXISTS (SELECT 1 FROM movies WHERE id = media_id AND deleted_at IS NULL)
			AND NOT EXISTS (SELECT 1 FROM episodes WHERE id = media_id)
			AND NOT EXISTS (SELECT 1 FROM series WHERE id = media_id)
			AND NOT EXISTS (SELECT 1 FROM tracks WHERE id = media_id)
			AND NOT EXISTS (SELECT 1 FROM artists WHERE id = media_id AND deleted_at IS NULL)
			THEN 1 ELSE 0 END
			FROM library_files WHERE library_id = ? ORDER BY path LIMIT ?`, []any{lib.ID}},
		{`SELECT f.file_path, 'movie:' || COALESCE(NULLIF(m.tmdb_id, ''), f.movie_id), f.quality || ':' || f.format,
			CASE WHEN m.id IS NULL OR m.deleted_at IS NOT NULL THEN 1 ELSE 0 END
			FROM movie_files f LEFT JOIN movies m ON m.id = f.movie_id
			WHERE f.deleted_at IS NULL AND substr(f.file_path, 1, ?) = ?
			ORDER BY f.file_path LIMIT ?`, []any{utf8.RuneCountInString(prefix), prefix}},
		{`SELECT f.file_path, 'episode:' || f.episode_id,
			COALESCE(f.quality, '') || ':' || COALESCE(f.resolution, '') || ':' || COALESCE(f.codec, ''),
			CASE WHEN e.id IS NULL OR s.id IS NULL THEN 1 ELSE 0 END
			FROM episode_files f LEFT JOIN episodes e ON e.id = f.episode_id
			LEFT JOIN series s ON s.id = f.series_id
			WHERE substr(f.file_path, 1, ?) = ? ORDER BY f.file_path LIMIT ?`, []any{utf8.RuneCountInString(prefix), prefix}},
		{`SELECT f.file_path, 'track:' || COALESCE(f.track_id, ''),
			f.quality || ':' || f.format,
			CASE WHEN t.id IS NULL OR a.id IS NULL OR a.deleted_at IS NOT NULL THEN 1 ELSE 0 END
			FROM track_files f LEFT JOIN tracks t ON t.id = f.track_id
			LEFT JOIN artists a ON a.id = f.artist_id
			WHERE f.deleted_at IS NULL AND substr(f.file_path, 1, ?) = ?
			ORDER BY f.file_path LIMIT ?`, []any{utf8.RuneCountInString(prefix), prefix}},
	}
	records := make([]healthRecord, 0)
	for _, query := range queries {
		query.args = append(query.args, limit-len(records)+1)
		rows, err := s.db.QueryContext(ctx, query.sql, query.args...)
		if err != nil {
			return nil, false, err
		}
		for rows.Next() {
			var record healthRecord
			if err := rows.Scan(&record.path, &record.group, &record.version, &record.orphan); err != nil {
				_ = rows.Close()
				return nil, false, err
			}
			if len(records) == limit {
				_ = rows.Close()
				return records, true, nil
			}
			records = append(records, record)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, false, err
		}
	}
	return records, false, nil
}

func (s *Store) saveHealth(ctx context.Context, report *HealthReport) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO library_health_reports (library_id, report)
		VALUES (?, ?) ON CONFLICT(library_id) DO UPDATE SET report = excluded.report`, report.LibraryID, string(data))
	return err
}

func (s *Store) loadHealth(ctx context.Context, libraryID string) (*HealthReport, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT report FROM library_health_reports WHERE library_id = ?`, libraryID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var report HealthReport
	if err := json.Unmarshal([]byte(data), &report); err != nil {
		return nil, fmt.Errorf("decode library health: %w", err)
	}
	return &report, s.applyHealthDismissals(ctx, &report)
}

func (s *Store) applyHealthDismissals(ctx context.Context, report *HealthReport) error {
	if len(report.Findings) == 0 {
		return nil
	}
	args := []any{report.LibraryID}
	for _, finding := range report.Findings {
		args = append(args, finding.ID)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(report.Findings)), ",")
	// Only generated placeholders are interpolated; all IDs remain parameters.
	rows, err := s.db.QueryContext(ctx, `SELECT finding_id FROM library_health_dismissals WHERE library_id = ? AND finding_id IN (`+placeholders+`)`, args...) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	dismissed := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		dismissed[id] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range report.Findings {
		report.Findings[i].Dismissed = dismissed[report.Findings[i].ID]
	}
	return nil
}

func healthWithin(root, path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
