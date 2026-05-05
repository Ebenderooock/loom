package anime

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
)

// Store handles SQLite persistence for anime preferences and episode mappings.
type Store struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewStore creates a Store backed by the given *sql.DB.
func NewStore(db *sql.DB, logger *slog.Logger) *Store {
	if logger == nil {
		logger = slog.Default()
	}
	return &Store{db: db, logger: logger}
}

// --- Preferences CRUD ---

// GetPreferences returns the anime preferences for a series, or defaults.
func (s *Store) GetPreferences(ctx context.Context, seriesID string) (*AnimePreferences, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT series_id, numbering_scheme, preferred_groups, dual_audio_required, release_group_scoring
		FROM anime_preferences WHERE series_id = ?`, seriesID)

	p := &AnimePreferences{SeriesID: seriesID}
	var groupsJSON, scoringJSON sql.NullString
	err := row.Scan(&p.SeriesID, &p.NumberingScheme, &groupsJSON, &p.DualAudioRequired, &scoringJSON)
	if err == sql.ErrNoRows {
		// Return sensible defaults
		p.NumberingScheme = NumberingAbsolute
		return p, nil
	}
	if err != nil {
		return nil, err
	}

	if groupsJSON.Valid {
		_ = json.Unmarshal([]byte(groupsJSON.String), &p.PreferredGroups)
	}
	if scoringJSON.Valid {
		_ = json.Unmarshal([]byte(scoringJSON.String), &p.ReleaseGroupScoring)
	}
	return p, nil
}

// UpsertPreferences creates or updates anime preferences for a series.
func (s *Store) UpsertPreferences(ctx context.Context, p *AnimePreferences) error {
	groupsJSON, _ := json.Marshal(p.PreferredGroups)
	scoringJSON, _ := json.Marshal(p.ReleaseGroupScoring)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO anime_preferences (series_id, numbering_scheme, preferred_groups, dual_audio_required, release_group_scoring)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(series_id) DO UPDATE SET
			numbering_scheme = excluded.numbering_scheme,
			preferred_groups = excluded.preferred_groups,
			dual_audio_required = excluded.dual_audio_required,
			release_group_scoring = excluded.release_group_scoring`,
		p.SeriesID, p.NumberingScheme, string(groupsJSON), p.DualAudioRequired, string(scoringJSON))
	return err
}

// DeletePreferences removes anime preferences for a series.
func (s *Store) DeletePreferences(ctx context.Context, seriesID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM anime_preferences WHERE series_id = ?`, seriesID)
	return err
}

// --- Mappings CRUD ---

// GetMappings returns all episode mappings for a series ordered by absolute number.
func (s *Store) GetMappings(ctx context.Context, seriesID string) ([]EpisodeMapping, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT absolute_number, season_number, episode_number
		FROM anime_mappings WHERE series_id = ?
		ORDER BY absolute_number`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EpisodeMapping
	for rows.Next() {
		var m EpisodeMapping
		if err := rows.Scan(&m.AbsoluteNumber, &m.SeasonNumber, &m.EpisodeNumber); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReplaceMappings atomically replaces all episode mappings for a series.
func (s *Store) ReplaceMappings(ctx context.Context, seriesID string, mappings []EpisodeMapping) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM anime_mappings WHERE series_id = ?`, seriesID); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO anime_mappings (series_id, absolute_number, season_number, episode_number)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range mappings {
		if _, err := stmt.ExecContext(ctx, seriesID, m.AbsoluteNumber, m.SeasonNumber, m.EpisodeNumber); err != nil {
			return err
		}
	}
	return tx.Commit()
}
