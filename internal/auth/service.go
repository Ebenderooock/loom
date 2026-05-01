package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/loomctl/loom/internal/storage"
	dbpg "github.com/loomctl/loom/internal/storage/db/postgres"
	dbsqlite "github.com/loomctl/loom/internal/storage/db/sqlite"
)

// ServiceOptions configures the auth Service.
type ServiceOptions struct {
	Store         Store
	Logger        *slog.Logger
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
		return SQLiteStore{Q: dbsqlite.New(db.DB())}, nil
	case storage.EnginePostgres:
		return PostgresStore{Q: dbpg.New(db.DB())}, nil
	default:
		return nil, errors.New("auth: unknown storage engine")
	}
}
