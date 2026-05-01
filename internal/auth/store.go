package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	dbpg "github.com/loomctl/loom/internal/storage/db/postgres"
	dbsqlite "github.com/loomctl/loom/internal/storage/db/sqlite"
)

// User is the engine-agnostic user row used by auth flows.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Email        string
	Role         string
}

// APIKey is the engine-agnostic API key row.
type APIKey struct {
	ID         int64
	UserID     int64
	Name       string
	KeyHash    string
	Prefix     string
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// CreateUserParams carries a new user's persisted columns.
type CreateUserParams struct {
	Username     string
	PasswordHash string
	Email        string
	Role         string
}

// CreateAPIKeyParams carries a new API key row.
type CreateAPIKeyParams struct {
	UserID    int64
	Name      string
	KeyHash   string
	Prefix    string
	ExpiresAt *time.Time
}

// Store is the storage seam consumed by the auth Service.
type Store interface {
	CreateUser(ctx context.Context, arg CreateUserParams) (User, error)
	GetUserByUsername(ctx context.Context, username string) (User, error)
	GetUserByID(ctx context.Context, id int64) (User, error)
	CountUsers(ctx context.Context) (int64, error)
	UpdateUserPassword(ctx context.Context, id int64, hash string) error
	UpdateUserOIDC(ctx context.Context, id int64, email, role string) (User, error)

	CreateAPIKey(ctx context.Context, arg CreateAPIKeyParams) (APIKey, error)
	GetAPIKeyByHash(ctx context.Context, hash string) (APIKey, error)
	ListAPIKeysForUser(ctx context.Context, userID int64) ([]APIKey, error)
	RevokeAPIKey(ctx context.Context, id, userID int64) error
	TouchAPIKey(ctx context.Context, id int64) error

	GetMeta(ctx context.Context, key string) (string, error)
	SetMeta(ctx context.Context, key, value string) error
}

// ErrNoRows wraps not-found lookups across both engines.
var ErrNoRows = sql.ErrNoRows

// SQLiteStore adapts dbsqlite.Queries to Store.
type SQLiteStore struct{ Q *dbsqlite.Queries }

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func sqliteUser(u dbsqlite.User) User {
	return User{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		Email:        u.Email.String,
		Role:         u.Role,
	}
}

func sqliteAPIKey(k dbsqlite.ApiKey) APIKey {
	out := APIKey{
		ID:        k.ID,
		UserID:    k.UserID,
		Name:      k.Name,
		KeyHash:   k.KeyHash,
		Prefix:    k.Prefix,
		CreatedAt: k.CreatedAt,
	}
	if k.LastUsedAt.Valid {
		t := k.LastUsedAt.Time
		out.LastUsedAt = &t
	}
	if k.ExpiresAt.Valid {
		t := k.ExpiresAt.Time
		out.ExpiresAt = &t
	}
	return out
}

func (s SQLiteStore) CreateUser(ctx context.Context, arg CreateUserParams) (User, error) {
	u, err := s.Q.CreateUser(ctx, dbsqlite.CreateUserParams{
		Username:     arg.Username,
		PasswordHash: arg.PasswordHash,
		Email:        nullStr(arg.Email),
		Role:         arg.Role,
	})
	if err != nil {
		return User{}, err
	}
	return sqliteUser(u), nil
}

func (s SQLiteStore) GetUserByUsername(ctx context.Context, username string) (User, error) {
	u, err := s.Q.GetUserByUsername(ctx, username)
	if err != nil {
		return User{}, err
	}
	return sqliteUser(u), nil
}

func (s SQLiteStore) GetUserByID(ctx context.Context, id int64) (User, error) {
	u, err := s.Q.GetUserByID(ctx, id)
	if err != nil {
		return User{}, err
	}
	return sqliteUser(u), nil
}

func (s SQLiteStore) CountUsers(ctx context.Context) (int64, error) {
	return s.Q.CountUsers(ctx)
}

func (s SQLiteStore) UpdateUserPassword(ctx context.Context, id int64, hash string) error {
	return s.Q.UpdateUserPassword(ctx, dbsqlite.UpdateUserPasswordParams{
		ID:           id,
		PasswordHash: hash,
	})
}

func (s SQLiteStore) UpdateUserOIDC(ctx context.Context, id int64, email, role string) (User, error) {
	u, err := s.Q.UpdateUserOIDC(ctx, dbsqlite.UpdateUserOIDCParams{
		ID:    id,
		Email: nullStr(email),
		Role:  role,
	})
	if err != nil {
		return User{}, err
	}
	return sqliteUser(u), nil
}

func (s SQLiteStore) CreateAPIKey(ctx context.Context, arg CreateAPIKeyParams) (APIKey, error) {
	k, err := s.Q.CreateAPIKey(ctx, dbsqlite.CreateAPIKeyParams{
		UserID:    arg.UserID,
		Name:      arg.Name,
		KeyHash:   arg.KeyHash,
		Prefix:    arg.Prefix,
		ExpiresAt: nullTime(arg.ExpiresAt),
	})
	if err != nil {
		return APIKey{}, err
	}
	return sqliteAPIKey(k), nil
}

func (s SQLiteStore) GetAPIKeyByHash(ctx context.Context, hash string) (APIKey, error) {
	k, err := s.Q.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		return APIKey{}, err
	}
	return sqliteAPIKey(k), nil
}

func (s SQLiteStore) ListAPIKeysForUser(ctx context.Context, userID int64) ([]APIKey, error) {
	rows, err := s.Q.ListAPIKeysForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]APIKey, 0, len(rows))
	for _, r := range rows {
		out = append(out, sqliteAPIKey(r))
	}
	return out, nil
}

func (s SQLiteStore) RevokeAPIKey(ctx context.Context, id, userID int64) error {
	return s.Q.RevokeAPIKey(ctx, dbsqlite.RevokeAPIKeyParams{ID: id, UserID: userID})
}

func (s SQLiteStore) TouchAPIKey(ctx context.Context, id int64) error {
	return s.Q.TouchAPIKey(ctx, id)
}

func (s SQLiteStore) GetMeta(ctx context.Context, key string) (string, error) {
	v, err := s.Q.GetSchemaMeta(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s SQLiteStore) SetMeta(ctx context.Context, key, value string) error {
	return s.Q.SetSchemaMeta(ctx, dbsqlite.SetSchemaMetaParams{Key: key, Value: value})
}

// PostgresStore adapts dbpg.Queries to Store.
type PostgresStore struct{ Q *dbpg.Queries }

func pgUser(u dbpg.User) User {
	return User{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		Email:        u.Email.String,
		Role:         u.Role,
	}
}

func pgAPIKey(k dbpg.ApiKey) APIKey {
	out := APIKey{
		ID:        k.ID,
		UserID:    k.UserID,
		Name:      k.Name,
		KeyHash:   k.KeyHash,
		Prefix:    k.Prefix,
		CreatedAt: k.CreatedAt,
	}
	if k.LastUsedAt.Valid {
		t := k.LastUsedAt.Time
		out.LastUsedAt = &t
	}
	if k.ExpiresAt.Valid {
		t := k.ExpiresAt.Time
		out.ExpiresAt = &t
	}
	return out
}

func (s PostgresStore) CreateUser(ctx context.Context, arg CreateUserParams) (User, error) {
	u, err := s.Q.CreateUser(ctx, dbpg.CreateUserParams{
		Username:     arg.Username,
		PasswordHash: arg.PasswordHash,
		Email:        nullStr(arg.Email),
		Role:         arg.Role,
	})
	if err != nil {
		return User{}, err
	}
	return pgUser(u), nil
}

func (s PostgresStore) GetUserByUsername(ctx context.Context, username string) (User, error) {
	u, err := s.Q.GetUserByUsername(ctx, username)
	if err != nil {
		return User{}, err
	}
	return pgUser(u), nil
}

func (s PostgresStore) GetUserByID(ctx context.Context, id int64) (User, error) {
	u, err := s.Q.GetUserByID(ctx, id)
	if err != nil {
		return User{}, err
	}
	return pgUser(u), nil
}

func (s PostgresStore) CountUsers(ctx context.Context) (int64, error) {
	return s.Q.CountUsers(ctx)
}

func (s PostgresStore) UpdateUserPassword(ctx context.Context, id int64, hash string) error {
	return s.Q.UpdateUserPassword(ctx, dbpg.UpdateUserPasswordParams{
		ID:           id,
		PasswordHash: hash,
	})
}

func (s PostgresStore) UpdateUserOIDC(ctx context.Context, id int64, email, role string) (User, error) {
	u, err := s.Q.UpdateUserOIDC(ctx, dbpg.UpdateUserOIDCParams{
		ID:    id,
		Email: nullStr(email),
		Role:  role,
	})
	if err != nil {
		return User{}, err
	}
	return pgUser(u), nil
}

func (s PostgresStore) CreateAPIKey(ctx context.Context, arg CreateAPIKeyParams) (APIKey, error) {
	k, err := s.Q.CreateAPIKey(ctx, dbpg.CreateAPIKeyParams{
		UserID:    arg.UserID,
		Name:      arg.Name,
		KeyHash:   arg.KeyHash,
		Prefix:    arg.Prefix,
		ExpiresAt: nullTime(arg.ExpiresAt),
	})
	if err != nil {
		return APIKey{}, err
	}
	return pgAPIKey(k), nil
}

func (s PostgresStore) GetAPIKeyByHash(ctx context.Context, hash string) (APIKey, error) {
	k, err := s.Q.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		return APIKey{}, err
	}
	return pgAPIKey(k), nil
}

func (s PostgresStore) ListAPIKeysForUser(ctx context.Context, userID int64) ([]APIKey, error) {
	rows, err := s.Q.ListAPIKeysForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]APIKey, 0, len(rows))
	for _, r := range rows {
		out = append(out, pgAPIKey(r))
	}
	return out, nil
}

func (s PostgresStore) RevokeAPIKey(ctx context.Context, id, userID int64) error {
	return s.Q.RevokeAPIKey(ctx, dbpg.RevokeAPIKeyParams{ID: id, UserID: userID})
}

func (s PostgresStore) TouchAPIKey(ctx context.Context, id int64) error {
	return s.Q.TouchAPIKey(ctx, id)
}

func (s PostgresStore) GetMeta(ctx context.Context, key string) (string, error) {
	v, err := s.Q.GetSchemaMeta(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s PostgresStore) SetMeta(ctx context.Context, key, value string) error {
	return s.Q.SetSchemaMeta(ctx, dbpg.SetSchemaMetaParams{Key: key, Value: value})
}
