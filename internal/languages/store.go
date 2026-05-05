package languages

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Store persists LanguageProfiles in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new Store backed by the given *sql.DB.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// List returns all language profiles ordered by name.
func (s *Store) List(ctx context.Context) ([]LanguageProfile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, languages, cutoff_language, upgrade_allowed,
		       created_at, updated_at
		FROM language_profiles ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("languages: list: %w", err)
	}
	defer rows.Close()

	var profiles []LanguageProfile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

// Get returns a single language profile by ID.
func (s *Store) Get(ctx context.Context, id string) (*LanguageProfile, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, languages, cutoff_language, upgrade_allowed,
		       created_at, updated_at
		FROM language_profiles WHERE id = ?`, id)

	var p LanguageProfile
	var langsJSON string
	var createdAt, updatedAt string
	err := row.Scan(&p.ID, &p.Name, &langsJSON, &p.CutoffLanguage,
		&p.UpgradeAllowed, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("language profile not found: %s", id)
		}
		return nil, fmt.Errorf("languages: get: %w", err)
	}
	if err := json.Unmarshal([]byte(langsJSON), &p.Languages); err != nil {
		return nil, fmt.Errorf("languages: unmarshal: %w", err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &p, nil
}

// Create inserts a new language profile. The ID must be set by the caller.
func (s *Store) Create(ctx context.Context, p *LanguageProfile) error {
	langsJSON, err := json.Marshal(p.Languages)
	if err != nil {
		return fmt.Errorf("languages: marshal: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO language_profiles (id, name, languages, cutoff_language,
		                               upgrade_allowed, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, string(langsJSON), p.CutoffLanguage,
		p.UpgradeAllowed, now, now)
	if err != nil {
		return fmt.Errorf("languages: create: %w", err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, now)
	p.UpdatedAt = p.CreatedAt
	return nil
}

// Update replaces a language profile in-place.
func (s *Store) Update(ctx context.Context, p *LanguageProfile) error {
	langsJSON, err := json.Marshal(p.Languages)
	if err != nil {
		return fmt.Errorf("languages: marshal: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		UPDATE language_profiles
		SET name = ?, languages = ?, cutoff_language = ?,
		    upgrade_allowed = ?, updated_at = ?
		WHERE id = ?`,
		p.Name, string(langsJSON), p.CutoffLanguage,
		p.UpgradeAllowed, now, p.ID)
	if err != nil {
		return fmt.Errorf("languages: update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("language profile not found: %s", p.ID)
	}
	p.UpdatedAt, _ = time.Parse(time.RFC3339, now)
	return nil
}

// Delete removes a language profile by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM language_profiles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("languages: delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("language profile not found: %s", id)
	}
	return nil
}

// EnsureDefault creates the "English" profile if no profiles exist.
func (s *Store) EnsureDefault(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM language_profiles`).Scan(&count); err != nil {
		return fmt.Errorf("languages: count: %w", err)
	}
	if count > 0 {
		return nil
	}
	eng, _ := LookupLanguage("en")
	return s.Create(ctx, &LanguageProfile{
		ID:   "english",
		Name: "English",
		Languages: []LanguagePriority{
			{Language: eng, Allowed: true, Priority: 1},
		},
		CutoffLanguage: "en",
		UpgradeAllowed: true,
	})
}

// ---------- helpers ----------

func scanProfile(rows *sql.Rows) (LanguageProfile, error) {
	var p LanguageProfile
	var langsJSON string
	var createdAt, updatedAt string
	if err := rows.Scan(&p.ID, &p.Name, &langsJSON, &p.CutoffLanguage,
		&p.UpgradeAllowed, &createdAt, &updatedAt); err != nil {
		return p, fmt.Errorf("languages: scan: %w", err)
	}
	if err := json.Unmarshal([]byte(langsJSON), &p.Languages); err != nil {
		return p, fmt.Errorf("languages: unmarshal: %w", err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return p, nil
}

// Slugify produces a URL-safe ID from a profile name.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, s)
	// collapse runs of dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
