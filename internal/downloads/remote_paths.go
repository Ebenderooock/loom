package downloads

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RemotePathMapping maps a download-client-reported path prefix to a
// locally accessible path. This allows Loom to operate in Docker or
// remote setups where the download client sees a different filesystem.
type RemotePathMapping struct {
	ID         string `json:"id"`
	ClientID   string `json:"client_id"`
	RemotePath string `json:"remote_path"`
	LocalPath  string `json:"local_path"`
	CreatedAt  string `json:"created_at"`
}

// RemotePathStore persists and queries remote path mappings.
type RemotePathStore struct {
	db *sql.DB
}

// NewRemotePathStore creates a store backed by the given database.
func NewRemotePathStore(db *sql.DB) *RemotePathStore {
	return &RemotePathStore{db: db}
}

// List returns all remote path mappings.
func (s *RemotePathStore) List(ctx context.Context) ([]RemotePathMapping, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, client_id, remote_path, local_path, created_at FROM remote_path_mappings ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []RemotePathMapping
	for rows.Next() {
		var m RemotePathMapping
		if err := rows.Scan(&m.ID, &m.ClientID, &m.RemotePath, &m.LocalPath, &m.CreatedAt); err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

// ListByClient returns mappings for a specific download client.
func (s *RemotePathStore) ListByClient(ctx context.Context, clientID string) ([]RemotePathMapping, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, client_id, remote_path, local_path, created_at FROM remote_path_mappings WHERE client_id = ? ORDER BY created_at`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []RemotePathMapping
	for rows.Next() {
		var m RemotePathMapping
		if err := rows.Scan(&m.ID, &m.ClientID, &m.RemotePath, &m.LocalPath, &m.CreatedAt); err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

// Create inserts a new remote path mapping. If m.ID is empty, a UUID
// is generated.
func (s *RemotePathStore) Create(ctx context.Context, m RemotePathMapping) (RemotePathMapping, error) {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.CreatedAt == "" {
		m.CreatedAt = time.Now().UTC().Format("2006-01-02 15:04:05")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO remote_path_mappings (id, client_id, remote_path, local_path, created_at) VALUES (?, ?, ?, ?, ?)`,
		m.ID, m.ClientID, m.RemotePath, m.LocalPath, m.CreatedAt)
	if err != nil {
		return RemotePathMapping{}, err
	}
	return m, nil
}

// Delete removes a mapping by ID.
func (s *RemotePathStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM remote_path_mappings WHERE id = ?`, id)
	return err
}

// MapPath applies the first matching remote-path mapping for the given
// client to path. If no mapping matches, the path is returned unchanged.
func (s *RemotePathStore) MapPath(ctx context.Context, clientID, path string) string {
	mappings, err := s.ListByClient(ctx, clientID)
	if err != nil || len(mappings) == 0 {
		return path
	}

	for _, m := range mappings {
		remotePath := ensureTrailingSlash(m.RemotePath)
		localPath := ensureTrailingSlash(m.LocalPath)

		if strings.HasPrefix(path, remotePath) {
			return localPath + strings.TrimPrefix(path, remotePath)
		}
		// Also try without trailing slash for exact directory match
		remoteNoSlash := strings.TrimRight(m.RemotePath, "/\\")
		if path == remoteNoSlash || strings.HasPrefix(path, remoteNoSlash+"/") {
			localNoSlash := strings.TrimRight(m.LocalPath, "/\\")
			return localNoSlash + strings.TrimPrefix(path, remoteNoSlash)
		}
	}
	return path
}

func ensureTrailingSlash(p string) string {
	if p == "" {
		return p
	}
	if !strings.HasSuffix(p, "/") && !strings.HasSuffix(p, "\\") {
		return p + "/"
	}
	return p
}
