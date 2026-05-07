package syncprofiles

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store persists sync profiles in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new sync profile store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new sync profile with its indexers and categories.
func (s *Store) Create(ctx context.Context, p *SyncProfile) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO sync_profiles (id, name, app_type, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.AppType, p.Enabled,
		p.CreatedAt.Format(time.RFC3339), p.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert sync_profile: %w", err)
	}

	if err := s.writeIndexers(ctx, tx, p.ID, p.Indexers); err != nil {
		return err
	}
	if err := s.writeCategories(ctx, tx, p.ID, p.Categories); err != nil {
		return err
	}

	return tx.Commit()
}

// Get returns a sync profile by ID with its indexers and categories.
func (s *Store) Get(ctx context.Context, id string) (*SyncProfile, error) {
	p := &SyncProfile{}
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, app_type, enabled, created_at, updated_at
		 FROM sync_profiles WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.AppType, &p.Enabled, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("sync profile not found: %s", id)
		}
		return nil, fmt.Errorf("get sync_profile: %w", err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	if p.Indexers, err = s.loadIndexers(ctx, id); err != nil {
		return nil, err
	}
	if p.Categories, err = s.loadCategories(ctx, id); err != nil {
		return nil, err
	}
	return p, nil
}

// List returns all sync profiles (without child indexer/category rows
// for performance; use Get for full detail).
func (s *Store) List(ctx context.Context) ([]*SyncProfile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, app_type, enabled, created_at, updated_at
		 FROM sync_profiles ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list sync_profiles: %w", err)
	}
	defer rows.Close()

	var profiles []*SyncProfile
	for rows.Next() {
		p := &SyncProfile{}
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.AppType, &p.Enabled, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan sync_profile: %w", err)
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

// Update patches a sync profile and replaces its indexers/categories.
func (s *Store) Update(ctx context.Context, p *SyncProfile) error {
	p.UpdatedAt = time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		`UPDATE sync_profiles
		 SET name = ?, app_type = ?, enabled = ?, updated_at = ?
		 WHERE id = ?`,
		p.Name, p.AppType, p.Enabled,
		p.UpdatedAt.Format(time.RFC3339), p.ID)
	if err != nil {
		return fmt.Errorf("update sync_profile: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sync profile not found: %s", p.ID)
	}

	// Replace child rows.
	if _, err := tx.ExecContext(ctx, `DELETE FROM sync_profile_indexers WHERE profile_id = ?`, p.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sync_profile_categories WHERE profile_id = ?`, p.ID); err != nil {
		return err
	}
	if err := s.writeIndexers(ctx, tx, p.ID, p.Indexers); err != nil {
		return err
	}
	if err := s.writeCategories(ctx, tx, p.ID, p.Categories); err != nil {
		return err
	}

	return tx.Commit()
}

// Delete removes a sync profile by ID (cascades to child tables).
func (s *Store) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sync_profiles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete sync_profile: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sync profile not found: %s", id)
	}
	return nil
}

// FilteredIndexerIDs returns the enabled indexer IDs for a profile.
// If the profile is disabled or not found, it returns nil, nil.
func (s *Store) FilteredIndexerIDs(ctx context.Context, profileID string) ([]string, error) {
	// Check profile exists and is enabled.
	var enabled bool
	err := s.db.QueryRowContext(ctx,
		`SELECT enabled FROM sync_profiles WHERE id = ?`, profileID,
	).Scan(&enabled)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if !enabled {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT indexer_id FROM sync_profile_indexers
		 WHERE profile_id = ? AND enabled = 1`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// --- helpers ---

func (s *Store) writeIndexers(ctx context.Context, tx *sql.Tx, profileID string, indexers []SyncProfileIndexer) error {
	for _, idx := range indexers {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO sync_profile_indexers (profile_id, indexer_id, enabled)
			 VALUES (?, ?, ?)`,
			profileID, idx.IndexerID, idx.Enabled)
		if err != nil {
			return fmt.Errorf("insert sync_profile_indexer: %w", err)
		}
	}
	return nil
}

func (s *Store) writeCategories(ctx context.Context, tx *sql.Tx, profileID string, cats []SyncProfileCategory) error {
	for _, c := range cats {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO sync_profile_categories (profile_id, category, mapped_to)
			 VALUES (?, ?, ?)`,
			profileID, c.Category, c.MappedTo)
		if err != nil {
			return fmt.Errorf("insert sync_profile_category: %w", err)
		}
	}
	return nil
}

func (s *Store) loadIndexers(ctx context.Context, profileID string) ([]SyncProfileIndexer, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT indexer_id, enabled FROM sync_profile_indexers WHERE profile_id = ?`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SyncProfileIndexer
	for rows.Next() {
		var si SyncProfileIndexer
		if err := rows.Scan(&si.IndexerID, &si.Enabled); err != nil {
			return nil, err
		}
		out = append(out, si)
	}
	return out, rows.Err()
}

func (s *Store) loadCategories(ctx context.Context, profileID string) ([]SyncProfileCategory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT category, mapped_to FROM sync_profile_categories WHERE profile_id = ?`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SyncProfileCategory
	for rows.Next() {
		var sc SyncProfileCategory
		if err := rows.Scan(&sc.Category, &sc.MappedTo); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}
