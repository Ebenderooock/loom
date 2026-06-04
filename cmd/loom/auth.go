package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

// buildAuthStore creates an auth Store from a storage.DB.
func buildAuthStore(db storage.DB) (auth.Store, error) {
	return auth.StoreFromDB(db)
}

// buildAuthService composes an *auth.Service from cfg + an opened storage
// connection. It loads (or generates) the session secret out of
// schema_meta and wires the OIDC + reverse-proxy helpers.
func buildAuthService(ctx context.Context, cfg *config.Config, db storage.DB, appCfg *appconfig.Config, appCfgPath string, logger *slog.Logger) (*auth.Service, error) {
	store, err := auth.StoreFromDB(db)
	if err != nil {
		return nil, err
	}
	secret, err := auth.LoadOrCreateSessionSecret(ctx, store, cfg.Auth.SessionSecret, logger)
	if err != nil {
		return nil, err
	}
	oidcCfg, proxyCfg := auth.FromConfig(cfg.Auth)
	var oidc *auth.OIDC
	if oidcCfg.Enabled {
		oidc = auth.NewOIDC(oidcCfg)
	}
	ttl := 30 * 24 * time.Hour
	if cfg.Auth.SessionTTL > 0 {
		ttl = time.Duration(cfg.Auth.SessionTTL) * time.Second
	}
	return auth.NewService(auth.ServiceOptions{
		Store:         store,
		Logger:        logger,
		AppConfig:     appCfg,
		AppConfigPath: appCfgPath,
		SessionSecret: secret,
		SessionTTL:    ttl,
		CookieSecure:  cfg.Auth.CookieSecure,
		OIDC:          oidc,
		Proxy:         proxyCfg,
		Invites:       auth.NewInviteStore(db.DB()),
	})
}
