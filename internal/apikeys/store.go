package apikeys

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Store provides SQLite persistence for automation API keys.
type Store struct {
	db *sql.DB
}

// NewStore wraps a *sql.DB.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// List returns all API keys.
func (s *Store) List(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, key, scopes, expires_at, last_used, created_at
		 FROM api_keys_v2 ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.Key, &k.Scopes, &k.ExpiresAt, &k.LastUsed, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// Get returns a single key by ID.
func (s *Store) Get(ctx context.Context, id string) (*APIKey, error) {
	var k APIKey
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, key, scopes, expires_at, last_used, created_at
		 FROM api_keys_v2 WHERE id = ?`, id).
		Scan(&k.ID, &k.Name, &k.Key, &k.Scopes, &k.ExpiresAt, &k.LastUsed, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// Create inserts a new API key and returns it with the generated key value.
func (s *Store) Create(ctx context.Context, name, scopes string, expiresAt *time.Time) (*APIKey, string, error) {
	id := generateID()
	key := "loom_" + generateHex(24)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys_v2 (id, name, key, scopes, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, name, key, scopes, expiresAt)
	if err != nil {
		return nil, "", fmt.Errorf("create api key: %w", err)
	}

	k, err := s.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	return k, key, nil
}

// Delete removes a key by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_keys_v2 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ValidateKey checks if a key string is valid and not expired.
// Returns the APIKey on success.
func (s *Store) ValidateKey(ctx context.Context, key string) (*APIKey, error) {
	var k APIKey
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, key, scopes, expires_at, last_used, created_at
		 FROM api_keys_v2 WHERE key = ?`, key).
		Scan(&k.ID, &k.Name, &k.Key, &k.Scopes, &k.ExpiresAt, &k.LastUsed, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return nil, fmt.Errorf("api key expired")
	}
	// Touch last_used
	go func() {
		now := time.Now()
		_, _ = s.db.ExecContext(context.Background(),
			`UPDATE api_keys_v2 SET last_used = ? WHERE id = ?`, now, k.ID)
	}()
	return &k, nil
}

func generateID() string {
	return generateHex(16)
}

func generateHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
