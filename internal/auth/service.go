package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
	"github.com/ebenderooock/loom/internal/storage"
	dbpg "github.com/ebenderooock/loom/internal/storage/db/postgres"
	dbsqlite "github.com/ebenderooock/loom/internal/storage/db/sqlite"
)

// ServiceOptions configures the auth Service.
type ServiceOptions struct {
	Store         Store
	Logger        *slog.Logger
	AppConfig     *appconfig.Config
	AppConfigPath string
	SessionSecret []byte
	SessionTTL    time.Duration
	CookieSecure  bool
	OIDC          *OIDC
	Proxy         *ProxyAuth
}

// Service is the orchestrator: middleware, handlers, and CLI all use
// this struct.
type Service struct {
	store         Store
	logger        *slog.Logger
	appConfig     *appconfig.Config
	appConfigPath string
	sessionSecret []byte
	sessionTTL    time.Duration
	cookieSecure  bool
	oidc          *OIDC
	proxy         *ProxyAuth
}

// NewService validates options and returns the configured Service. The
// session secret must be at least 16 bytes.
func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Store == nil {
		return nil, errors.New("auth: store is required")
	}
	if opts.AppConfig == nil {
		return nil, errors.New("auth: appConfig is required")
	}
	if opts.AppConfigPath == "" {
		return nil, errors.New("auth: appConfigPath is required")
	}
	if len(opts.SessionSecret) < 16 {
		return nil, errors.New("auth: session secret must be >= 16 bytes")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.SessionTTL <= 0 {
		opts.SessionTTL = 30 * 24 * time.Hour
	}
	return &Service{
		store:         opts.Store,
		logger:        opts.Logger,
		appConfig:     opts.AppConfig,
		appConfigPath: opts.AppConfigPath,
		sessionSecret: opts.SessionSecret,
		sessionTTL:    opts.SessionTTL,
		cookieSecure:  opts.CookieSecure,
		oidc:          opts.OIDC,
		proxy:         opts.Proxy,
	}, nil
}

// Store returns the underlying Store. Used by CLI subcommands.
func (s *Service) Store() Store { return s.store }

// SessionSecret returns the active HMAC key. Test-only accessor.
func (s *Service) SessionSecret() []byte { return s.sessionSecret }

// OIDCConfigured reports whether the OIDC helper is wired in.
func (s *Service) OIDCConfigured() bool { return s.oidc != nil && s.oidc.cfg.Enabled }

// SchemaMetaSessionSecretKey is the schema_meta key under which a
// generated session secret is persisted across restarts.
const SchemaMetaSessionSecretKey = "auth.session_secret"

// LoadOrCreateSessionSecret returns the configured secret if non-empty.
// Otherwise it loads the persisted secret from schema_meta, generating a
// new one (and warning) if absent.
func LoadOrCreateSessionSecret(ctx context.Context, store Store, configured string, logger *slog.Logger) ([]byte, error) {
	if s := strings.TrimSpace(configured); s != "" {
		return []byte(s), nil
	}
	persisted, err := store.GetMeta(ctx, SchemaMetaSessionSecretKey)
	if err != nil {
		return nil, err
	}
	if persisted != "" {
		return base64.StdEncoding.DecodeString(persisted)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(buf)
	if err := store.SetMeta(ctx, SchemaMetaSessionSecretKey, encoded); err != nil {
		return nil, err
	}
	if logger != nil {
		logger.Warn("auth: generated session secret automatically; set auth.session_secret in config for stability across restarts")
	}
	return buf, nil
}

// StoreFromDB returns a Store backed by the given storage.DB, dispatching
// on engine. It's the single seam between the storage and auth packages.
func StoreFromDB(db storage.DB) (Store, error) {
	switch db.Engine() {
	case storage.EngineSQLite:
		return SQLiteStore{Q: dbsqlite.New(db.DB()), DB: db.DB()}, nil
	case storage.EnginePostgres:
		return PostgresStore{Q: dbpg.New(db.DB()), DB: db.DB()}, nil
	default:
		return nil, errors.New("auth: unknown storage engine")
	}
}

// schemaMetaAdminUserID tracks the DB row ID of the config-managed admin user.
const schemaMetaAdminUserID = "auth.admin_user_id"

// ReconcileAdmin ensures the database admin user matches the config credentials.
// It uses an admin user ID stored in schema_meta to track the config-managed row,
// so username changes in config are correctly applied to the same DB row.
// Returns the reconciled user. This is safe to call from both startup and handlers.
func (s *Service) ReconcileAdmin(ctx context.Context) (User, error) {
	if s.appConfig.Admin.Username == "" || s.appConfig.Admin.PasswordHash == "" {
		return User{}, errors.New("auth: admin credentials not configured")
	}

	// Check if we have a tracked admin user ID
	adminIDStr, err := s.store.GetMeta(ctx, schemaMetaAdminUserID)
	if err != nil {
		return User{}, err
	}

	if adminIDStr != "" {
		// We have a tracked admin — update username + password to match config
		var adminID int64
		if _, err := fmt.Sscanf(adminIDStr, "%d", &adminID); err != nil {
			return User{}, fmt.Errorf("auth: invalid admin user id in schema_meta: %w", err)
		}
		if err := s.store.UpdateUserAdmin(ctx, adminID, s.appConfig.Admin.Username, s.appConfig.Admin.PasswordHash); err != nil {
			return User{}, fmt.Errorf("auth: update admin user: %w", err)
		}
		// Force role back to admin: OIDC/proxy reconciliation could have
		// demoted the protected admin while it was offline.
		if err := s.store.UpdateUserRole(ctx, adminID, "admin"); err != nil {
			return User{}, fmt.Errorf("auth: restore admin role: %w", err)
		}
		u, err := s.store.GetUserByID(ctx, adminID)
		if err != nil {
			return User{}, fmt.Errorf("auth: get admin user after update: %w", err)
		}
		s.logger.Info("admin user reconciled from config", "username", u.Username, "id", u.ID)
		return u, nil
	}

	// No tracked admin — try to find by username first
	u, err := s.store.GetUserByUsername(ctx, s.appConfig.Admin.Username)
	if err == nil {
		// Found existing user, update password and track
		if u.PasswordHash != s.appConfig.Admin.PasswordHash {
			if err := s.store.UpdateUserPassword(ctx, u.ID, s.appConfig.Admin.PasswordHash); err != nil {
				return User{}, fmt.Errorf("auth: update admin password: %w", err)
			}
			u.PasswordHash = s.appConfig.Admin.PasswordHash
		}
		if err := s.store.SetMeta(ctx, schemaMetaAdminUserID, fmt.Sprintf("%d", u.ID)); err != nil {
			return User{}, fmt.Errorf("auth: save admin user id: %w", err)
		}
		if u.Role != "admin" {
			if err := s.store.UpdateUserRole(ctx, u.ID, "admin"); err != nil {
				return User{}, fmt.Errorf("auth: restore admin role: %w", err)
			}
			u.Role = "admin"
		}
		s.logger.Info("admin user tracked from existing user", "username", u.Username, "id", u.ID)
		return u, nil
	}
	if !errors.Is(err, ErrNoRows) {
		return User{}, fmt.Errorf("auth: lookup admin user: %w", err)
	}

	// No existing user — create new admin from config
	u, err = s.store.CreateUser(ctx, CreateUserParams{
		Username:     s.appConfig.Admin.Username,
		PasswordHash: s.appConfig.Admin.PasswordHash,
		Email:        "",
		Role:         "admin",
	})
	if err != nil {
		return User{}, fmt.Errorf("auth: create admin user: %w", err)
	}
	if err := s.store.SetMeta(ctx, schemaMetaAdminUserID, fmt.Sprintf("%d", u.ID)); err != nil {
		return User{}, fmt.Errorf("auth: save admin user id: %w", err)
	}
	s.logger.Info("admin user created from config", "username", u.Username, "id", u.ID)
	return u, nil
}
