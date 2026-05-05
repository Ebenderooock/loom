package customformats

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Store provides SQLite persistence for custom formats.
type Store struct {
	db *sql.DB
}

// NewStore wraps a *sql.DB for custom format CRUD.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// List returns all custom formats ordered by name.
func (s *Store) List(ctx context.Context) ([]CustomFormat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, include_when_renaming, specifications, score, created_at, updated_at
		 FROM custom_formats ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list custom formats: %w", err)
	}
	defer rows.Close()
	return scanFormats(rows)
}

// Get returns a single custom format by ID.
func (s *Store) Get(ctx context.Context, id string) (*CustomFormat, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, include_when_renaming, specifications, score, created_at, updated_at
		 FROM custom_formats WHERE id = ?`, id)
	return scanFormat(row)
}

// Create inserts a new custom format.
func (s *Store) Create(ctx context.Context, cf *CustomFormat) error {
	specJSON, err := json.Marshal(cf.Specifications)
	if err != nil {
		return fmt.Errorf("marshal specifications: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO custom_formats (id, name, include_when_renaming, specifications, score, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cf.ID, cf.Name, cf.IncludeWhenRenaming, string(specJSON), cf.Score, now, now)
	if err != nil {
		return fmt.Errorf("create custom format: %w", err)
	}
	cf.CreatedAt = now
	cf.UpdatedAt = now
	return nil
}

// Update replaces a custom format by ID.
func (s *Store) Update(ctx context.Context, cf *CustomFormat) error {
	specJSON, err := json.Marshal(cf.Specifications)
	if err != nil {
		return fmt.Errorf("marshal specifications: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE custom_formats SET name=?, include_when_renaming=?, specifications=?, score=?, updated_at=?
		 WHERE id=?`,
		cf.Name, cf.IncludeWhenRenaming, string(specJSON), cf.Score, now, cf.ID)
	if err != nil {
		return fmt.Errorf("update custom format: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	cf.UpdatedAt = now
	return nil
}

// Delete removes a custom format by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM custom_formats WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete custom format: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanFormats(rows *sql.Rows) ([]CustomFormat, error) {
	var out []CustomFormat
	for rows.Next() {
		var cf CustomFormat
		var specJSON string
		if err := rows.Scan(&cf.ID, &cf.Name, &cf.IncludeWhenRenaming, &specJSON, &cf.Score, &cf.CreatedAt, &cf.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan custom format: %w", err)
		}
		if err := json.Unmarshal([]byte(specJSON), &cf.Specifications); err != nil {
			return nil, fmt.Errorf("unmarshal specifications for %s: %w", cf.ID, err)
		}
		out = append(out, cf)
	}
	return out, rows.Err()
}

func scanFormat(row *sql.Row) (*CustomFormat, error) {
	var cf CustomFormat
	var specJSON string
	if err := row.Scan(&cf.ID, &cf.Name, &cf.IncludeWhenRenaming, &specJSON, &cf.Score, &cf.CreatedAt, &cf.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(specJSON), &cf.Specifications); err != nil {
		return nil, fmt.Errorf("unmarshal specifications for %s: %w", cf.ID, err)
	}
	return &cf, nil
}
