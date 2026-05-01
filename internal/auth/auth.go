// Package auth provides Loom's authentication subsystem: argon2id-hashed
// forms login with HMAC-signed session cookies, API keys, OIDC, and
// reverse-proxy header trust. The Service composes the four modes behind
// a single chi-friendly middleware (RequireAuth / RequireRole) and a set
// of HTTP handlers mounted under /api/v1/auth.
package auth

import (
	"context"
	"errors"
)

// Method names the mechanism that authenticated the caller.
type Method string

const (
	MethodSession Method = "session"
	MethodAPIKey  Method = "apikey"
	MethodOIDC    Method = "oidc"
	MethodProxy   Method = "proxy"
)

// Identity is the resolved caller for a request. Roles is normalized to
// lower-case strings; "admin" gates RequireRole.
type Identity struct {
	UserID     int64
	Username   string
	Email      string
	Roles      []string
	AuthMethod Method
}

// HasRole reports whether the identity has been granted role.
func (i *Identity) HasRole(role string) bool {
	if i == nil {
		return false
	}
	for _, r := range i.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type ctxKey int

const identityKey ctxKey = 1

// WithIdentity returns a copy of ctx carrying id.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// IdentityFrom returns the *Identity attached to ctx, or nil if absent.
func IdentityFrom(ctx context.Context) *Identity {
	v, _ := ctx.Value(identityKey).(*Identity)
	return v
}

// ErrUnauthenticated is returned by store helpers when no row matches.
var ErrUnauthenticated = errors.New("auth: unauthenticated")
