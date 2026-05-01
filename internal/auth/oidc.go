package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig is a copy of config.OIDCConfig with named fields. Defined
// locally so the auth package doesn't import config (and to keep the
// boundary narrow for testing).
type OIDCConfig struct {
	Enabled       bool
	IssuerURL     string
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	Scopes        []string
	UsernameClaim string
	EmailClaim    string
	RoleClaim     string
	AdminGroups   []string
}

// OIDC encapsulates the lazy-initialized provider, oauth2 config, and
// token verifier for the configured issuer.
type OIDC struct {
	cfg      OIDCConfig
	mu       sync.Mutex
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
}

// NewOIDC builds an OIDC helper. Discovery is deferred until first use.
func NewOIDC(cfg OIDCConfig) *OIDC {
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	if cfg.UsernameClaim == "" {
		cfg.UsernameClaim = "preferred_username"
	}
	if cfg.EmailClaim == "" {
		cfg.EmailClaim = "email"
	}
	if cfg.RoleClaim == "" {
		cfg.RoleClaim = "groups"
	}
	return &OIDC{cfg: cfg}
}

// Init eagerly performs discovery. Safe to call repeatedly; idempotent.
func (o *OIDC) Init(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider != nil {
		return nil
	}
	if !o.cfg.Enabled {
		return errors.New("auth: oidc disabled")
	}
	p, err := oidc.NewProvider(ctx, o.cfg.IssuerURL)
	if err != nil {
		return fmt.Errorf("auth: oidc discovery: %w", err)
	}
	o.provider = p
	o.verifier = p.Verifier(&oidc.Config{ClientID: o.cfg.ClientID})
	o.oauth = &oauth2.Config{
		ClientID:     o.cfg.ClientID,
		ClientSecret: o.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  o.cfg.RedirectURL,
		Scopes:       o.cfg.Scopes,
	}
	return nil
}

// SetVerifier overrides the lazy-discovered verifier and oauth2 config.
// Used by tests to inject a stubbed token issuer.
func (o *OIDC) SetVerifier(v *oidc.IDTokenVerifier, oc *oauth2.Config) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.verifier = v
	o.oauth = oc
}

func (o *OIDC) ensure(ctx context.Context) error {
	o.mu.Lock()
	ready := o.verifier != nil && o.oauth != nil
	o.mu.Unlock()
	if ready {
		return nil
	}
	return o.Init(ctx)
}

// AuthCodeURL returns the redirect URL plus the state value to plant in
// a cookie for CSRF defense.
func (o *OIDC) AuthCodeURL(ctx context.Context, state string, opts ...oauth2.AuthCodeOption) (string, error) {
	if err := o.ensure(ctx); err != nil {
		return "", err
	}
	return o.oauth.AuthCodeURL(state, opts...), nil
}

// Exchange swaps an authorization code for a token. The caller passes the
// code received on the callback URL.
func (o *OIDC) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	if err := o.ensure(ctx); err != nil {
		return nil, err
	}
	return o.oauth.Exchange(ctx, code, opts...)
}

// VerifyIDToken parses claims out of the id_token field of t.
func (o *OIDC) VerifyIDToken(ctx context.Context, t *oauth2.Token) (map[string]any, error) {
	if err := o.ensure(ctx); err != nil {
		return nil, err
	}
	raw, ok := t.Extra("id_token").(string)
	if !ok || raw == "" {
		return nil, errors.New("auth: oidc token missing id_token")
	}
	idt, err := o.verifier.Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("auth: verify id_token: %w", err)
	}
	claims := map[string]any{}
	if err := idt.Claims(&claims); err != nil {
		return nil, fmt.Errorf("auth: oidc claims: %w", err)
	}
	return claims, nil
}

// ClaimUsername extracts the configured username claim, with a fallback
// chain: configured → "preferred_username" → "sub" → "email".
func (o *OIDC) ClaimUsername(claims map[string]any) string {
	if v, ok := claims[o.cfg.UsernameClaim].(string); ok && v != "" {
		return v
	}
	if v, ok := claims["preferred_username"].(string); ok && v != "" {
		return v
	}
	if v, ok := claims["sub"].(string); ok && v != "" {
		return v
	}
	if v, ok := claims["email"].(string); ok && v != "" {
		return v
	}
	return ""
}

// ClaimEmail extracts the configured email claim.
func (o *OIDC) ClaimEmail(claims map[string]any) string {
	if v, ok := claims[o.cfg.EmailClaim].(string); ok {
		return v
	}
	return ""
}

// ClaimRole returns "admin" if any group in the role claim is in
// AdminGroups, else "user".
func (o *OIDC) ClaimRole(claims map[string]any) string {
	groups := claimStrings(claims, o.cfg.RoleClaim)
	for _, g := range groups {
		for _, admin := range o.cfg.AdminGroups {
			if g == admin {
				return "admin"
			}
		}
	}
	return "user"
}

func claimStrings(claims map[string]any, key string) []string {
	v, ok := claims[key]
	if !ok {
		return nil
	}
	switch vv := v.(type) {
	case string:
		out := []string{}
		for _, p := range strings.Split(vv, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(vv))
		for _, x := range vv {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// State cookie helpers — the OIDC flow uses an HMAC of (state||nonce)
// stamped into a cookie so we can verify the callback without server-side
// session storage.

const oidcStateCookie = "loom_oidc_state"

// SignOIDCState returns a value of "<stateB64>.<sigB64>" plus the random
// state itself for the auth-code redirect.
func SignOIDCState(secret []byte) (state, cookieValue string, err error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	state = base64.RawURLEncoding.EncodeToString(buf)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(state))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	cookieValue = state + "." + sig
	return state, cookieValue, nil
}

// VerifyOIDCState compares the state value posted back by the IdP to
// the signed cookie planted on the redirect.
func VerifyOIDCState(secret []byte, state, cookieValue string) error {
	parts := strings.SplitN(cookieValue, ".", 2)
	if len(parts) != 2 {
		return errors.New("auth: oidc state cookie malformed")
	}
	if parts[0] != state {
		return errors.New("auth: oidc state mismatch")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(state))
	expected := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(got, expected) {
		return errors.New("auth: oidc state signature invalid")
	}
	return nil
}

// IssueOIDCStateCookie writes the signed state cookie onto w.
func IssueOIDCStateCookie(w http.ResponseWriter, r *http.Request, value string, cookieSecure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    value,
		Path:     "/api/v1/auth/oidc",
		Expires:  time.Now().Add(10 * time.Minute),
		MaxAge:   600,
		HttpOnly: true,
		Secure:   cookieSecure || requestIsTLS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearOIDCStateCookie expires the OIDC state cookie.
func ClearOIDCStateCookie(w http.ResponseWriter, r *http.Request, cookieSecure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    "",
		Path:     "/api/v1/auth/oidc",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cookieSecure || requestIsTLS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// UpsertOIDCUser creates or updates a user from OIDC claims and returns
// the resulting auth-layer User. It uses GetUserByUsername + CreateUser /
// UpdateUserOIDC under the hood. Exposed for direct unit testing.
func UpsertOIDCUser(ctx context.Context, store Store, username, email, role string) (User, error) {
	if username == "" {
		return User{}, errors.New("auth: oidc username claim empty")
	}
	existing, err := store.GetUserByUsername(ctx, username)
	if err == nil {
		// Update if anything changed.
		if existing.Email != email || existing.Role != role {
			return store.UpdateUserOIDC(ctx, existing.ID, email, role)
		}
		return existing, nil
	}
	if !errors.Is(err, ErrNoRows) {
		return User{}, err
	}
	// Random throwaway password — OIDC users authenticate via IdP only.
	pw, err := randomBase62(32)
	if err != nil {
		return User{}, err
	}
	hash, err := HashPassword(pw)
	if err != nil {
		return User{}, err
	}
	return store.CreateUser(ctx, CreateUserParams{
		Username:     username,
		PasswordHash: hash,
		Email:        email,
		Role:         role,
	})
}

// jsonClaim is a helper used by debug logging / tests.
func jsonClaim(claims map[string]any) string {
	b, _ := json.Marshal(claims)
	return string(b)
}

var _ = jsonClaim // keep accessible for future debug paths
