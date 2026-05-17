package mediainfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
)

// Store handles SQLite persistence for media preferences.
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

// GetPreferences returns the media preferences (singleton row with id='default').
func (s *Store) GetPreferences(ctx context.Context) (*MediaPreferences, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, preferred_audio, preferred_sub_languages,
		       require_subtitles, prefer_hdr, prefer_atmos,
		       default_quality_profile_id,
		       created_at, updated_at
		FROM media_preferences WHERE id = 'default'`)

	p := &MediaPreferences{ID: "default"}
	var audioJSON, subJSON string
	var reqSub, hdr, atmos int
	err := row.Scan(&p.ID, &audioJSON, &subJSON, &reqSub, &hdr, &atmos, &p.DefaultQualityProfileID, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		// Return sensible defaults
		p.PreferredAudioCodecs = []string{}
		p.PreferredSubLanguages = []string{}
		p.PreferHDR = true
		p.PreferAtmos = true
		return p, nil
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(audioJSON), &p.PreferredAudioCodecs)
	_ = json.Unmarshal([]byte(subJSON), &p.PreferredSubLanguages)
	p.RequireSubtitles = reqSub != 0
	p.PreferHDR = hdr != 0
	p.PreferAtmos = atmos != 0

	if p.PreferredAudioCodecs == nil {
		p.PreferredAudioCodecs = []string{}
	}
	if p.PreferredSubLanguages == nil {
		p.PreferredSubLanguages = []string{}
	}
	return p, nil
}

// UpsertPreferences creates or updates the default media preferences row.
func (s *Store) UpsertPreferences(ctx context.Context, p *MediaPreferences) error {
	audioJSON, _ := json.Marshal(p.PreferredAudioCodecs)
	subJSON, _ := json.Marshal(p.PreferredSubLanguages)

	reqSub := 0
	if p.RequireSubtitles {
		reqSub = 1
	}
	hdr := 0
	if p.PreferHDR {
		hdr = 1
	}
	atmos := 0
	if p.PreferAtmos {
		atmos = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO media_preferences (id, preferred_audio, preferred_sub_languages, require_subtitles, prefer_hdr, prefer_atmos, default_quality_profile_id, updated_at)
		VALUES ('default', ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			preferred_audio = excluded.preferred_audio,
			preferred_sub_languages = excluded.preferred_sub_languages,
			require_subtitles = excluded.require_subtitles,
			prefer_hdr = excluded.prefer_hdr,
			prefer_atmos = excluded.prefer_atmos,
			default_quality_profile_id = excluded.default_quality_profile_id,
			updated_at = CURRENT_TIMESTAMP`,
		string(audioJSON), string(subJSON), reqSub, hdr, atmos, p.DefaultQualityProfileID)
	return err
}
