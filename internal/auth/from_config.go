package auth

import "github.com/ebenderooock/loom/internal/kernel/config"

// FromConfig converts the application config block to the auth-local
// OIDC and Proxy types so the auth package doesn't need a hard import
// edge with config beyond this single file.
func FromConfig(cfg config.AuthConfig) (OIDCConfig, *ProxyAuth) {
	oc := OIDCConfig{
		Enabled:       cfg.OIDC.Enabled,
		IssuerURL:     cfg.OIDC.IssuerURL,
		ClientID:      cfg.OIDC.ClientID,
		ClientSecret:  cfg.OIDC.ClientSecret,
		RedirectURL:   cfg.OIDC.RedirectURL,
		Scopes:        cfg.OIDC.Scopes,
		UsernameClaim: cfg.OIDC.UsernameClaim,
		EmailClaim:    cfg.OIDC.EmailClaim,
		RoleClaim:     cfg.OIDC.RoleClaim,
		AdminGroups:   cfg.OIDC.AdminGroups,
	}
	pa := NewProxyAuth(
		cfg.Proxy.Enabled,
		cfg.Proxy.TrustedCIDRs,
		cfg.Proxy.UserHeader,
		cfg.Proxy.EmailHeader,
		cfg.Proxy.GroupsHeader,
		cfg.OIDC.AdminGroups,
	)
	return oc, pa
}
