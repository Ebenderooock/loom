package qualityprofiles

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Store provides SQLite persistence for quality profiles.
type Store struct {
	db *sql.DB
}

// NewStore wraps a *sql.DB.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// List returns all quality profiles with their format items.
func (s *Store) List(ctx context.Context) ([]QualityProfile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, cutoff, min_format_score, cutoff_format_score, upgrade_allowed, items, created_at, updated_at
		 FROM quality_profiles_v2 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list quality profiles: %w", err)
	}
	defer rows.Close()

	var out []QualityProfile
	for rows.Next() {
		qp, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, qp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		items, err := s.listFormatItems(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].FormatItems = items
	}
	return out, nil
}

// Get returns a single profile by ID.
func (s *Store) Get(ctx context.Context, id string) (*QualityProfile, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, cutoff, min_format_score, cutoff_format_score, upgrade_allowed, items, created_at, updated_at
		 FROM quality_profiles_v2 WHERE id = ?`, id)
	qp, err := scanProfileRow(row)
	if err != nil {
		return nil, err
	}
	items, err := s.listFormatItems(ctx, qp.ID)
	if err != nil {
		return nil, err
	}
	qp.FormatItems = items
	return qp, nil
}

// Create inserts a new quality profile.
func (s *Store) Create(ctx context.Context, qp *QualityProfile) error {
	if qp.ID == "" {
		qp.ID = generateID()
	}
	now := time.Now().UTC()
	if qp.Items == "" {
		qp.Items = "[]"
	}
	upgradeInt := 0
	if qp.UpgradeAllowed {
		upgradeInt = 1
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO quality_profiles_v2 (id, name, cutoff, min_format_score, cutoff_format_score, upgrade_allowed, items, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		qp.ID, qp.Name, qp.Cutoff, qp.MinFormatScore, qp.CutoffFormatScore, upgradeInt, qp.Items, now, now)
	if err != nil {
		return fmt.Errorf("create quality profile: %w", err)
	}
	qp.CreatedAt = now
	qp.UpdatedAt = now

	return s.replaceFormatItems(ctx, qp.ID, qp.FormatItems)
}

// Update replaces a quality profile by ID.
func (s *Store) Update(ctx context.Context, qp *QualityProfile) error {
	now := time.Now().UTC()
	if qp.Items == "" {
		qp.Items = "[]"
	}
	upgradeInt := 0
	if qp.UpgradeAllowed {
		upgradeInt = 1
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE quality_profiles_v2
		 SET name=?, cutoff=?, min_format_score=?, cutoff_format_score=?, upgrade_allowed=?, items=?, updated_at=?
		 WHERE id=?`,
		qp.Name, qp.Cutoff, qp.MinFormatScore, qp.CutoffFormatScore, upgradeInt, qp.Items, now, qp.ID)
	if err != nil {
		return fmt.Errorf("update quality profile: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	qp.UpdatedAt = now

	return s.replaceFormatItems(ctx, qp.ID, qp.FormatItems)
}

// Delete removes a quality profile by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM quality_profiles_v2 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete quality profile: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetFormatScores returns the format items for a profile.
func (s *Store) GetFormatScores(ctx context.Context, profileID string) ([]FormatItem, error) {
	return s.listFormatItems(ctx, profileID)
}

// SetFormatScores replaces format scores for a profile.
func (s *Store) SetFormatScores(ctx context.Context, profileID string, items []FormatItem) error {
	// Verify profile exists
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM quality_profiles_v2 WHERE id = ?`, profileID).Scan(&exists)
	if err != nil {
		return sql.ErrNoRows
	}
	return s.replaceFormatItems(ctx, profileID, items)
}

func (s *Store) listFormatItems(ctx context.Context, profileID string) ([]FormatItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT profile_id, format_id, score
		 FROM quality_profile_format_items WHERE profile_id = ? ORDER BY format_id`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list format items: %w", err)
	}
	defer rows.Close()

	var out []FormatItem
	for rows.Next() {
		var fi FormatItem
		if err := rows.Scan(&fi.ProfileID, &fi.FormatID, &fi.Score); err != nil {
			return nil, fmt.Errorf("scan format item: %w", err)
		}
		out = append(out, fi)
	}
	return out, rows.Err()
}

func (s *Store) replaceFormatItems(ctx context.Context, profileID string, items []FormatItem) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM quality_profile_format_items WHERE profile_id = ?`, profileID)
	if err != nil {
		return fmt.Errorf("clear format items: %w", err)
	}
	for _, fi := range items {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO quality_profile_format_items (profile_id, format_id, score) VALUES (?, ?, ?)`,
			profileID, fi.FormatID, fi.Score)
		if err != nil {
			return fmt.Errorf("insert format item: %w", err)
		}
	}
	return nil
}

func scanProfile(rows *sql.Rows) (QualityProfile, error) {
	var qp QualityProfile
	var upgradeInt int
	if err := rows.Scan(&qp.ID, &qp.Name, &qp.Cutoff, &qp.MinFormatScore, &qp.CutoffFormatScore, &upgradeInt, &qp.Items, &qp.CreatedAt, &qp.UpdatedAt); err != nil {
		return QualityProfile{}, fmt.Errorf("scan quality profile: %w", err)
	}
	qp.UpgradeAllowed = upgradeInt != 0
	return qp, nil
}

func scanProfileRow(row *sql.Row) (*QualityProfile, error) {
	var qp QualityProfile
	var upgradeInt int
	if err := row.Scan(&qp.ID, &qp.Name, &qp.Cutoff, &qp.MinFormatScore, &qp.CutoffFormatScore, &upgradeInt, &qp.Items, &qp.CreatedAt, &qp.UpdatedAt); err != nil {
		return nil, err
	}
	qp.UpgradeAllowed = upgradeInt != 0
	return &qp, nil
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
