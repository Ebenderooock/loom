package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// RequireAuth returns chi-compatible middleware that resolves the caller
// against the configured modes (proxy → api-key → session). Unauthorized
// requests get 401 with a JSON body.
func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := s.resolveIdentity(r)
		if err != nil || id == nil {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
	})
}

// RequireRole composes RequireAuth and additionally requires that the
// resolved identity has the given role.
func (s *Service) RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return s.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := IdentityFrom(r.Context())
			if id == nil || !id.HasRole(role) {
				writeAuthError(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
}

func (s *Service) resolveIdentity(r *http.Request) (*Identity, error) {
	ctx := r.Context()

	// 1) Reverse-proxy headers.
	if s.proxy != nil && s.proxy.IsTrusted(r) {
		if id := s.proxy.HeaderIdentity(r); id != nil {
			user, err := s.upsertProxyUser(ctx, id)
			if err != nil {
				return nil, err
			}
			id.UserID = user.ID
			id.Roles = appendUnique(id.Roles, user.Role)
			return id, nil
		}
	}

	// 2) API key.
	if key := extractAPIKey(r); key != "" {
		if id, err := s.identityFromAPIKey(ctx, key); err == nil {
			return id, nil
		} else if !errors.Is(err, ErrUnauthenticated) {
			return nil, err
		}
	}

	// 3) Session cookie.
	if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
		if id, err := s.identityFromSession(ctx, c.Value); err == nil {
			return id, nil
		}
	}

	return nil, ErrUnauthenticated
}

func (s *Service) identityFromSession(ctx context.Context, cookieValue string) (*Identity, error) {
	payload, err := VerifySession(s.sessionSecret, cookieValue, time.Now())
	if err != nil {
		return nil, err
	}
	u, err := s.store.GetUserByID(ctx, payload.UID)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	return &Identity{
		UserID:     u.ID,
		Username:   u.Username,
		Email:      u.Email,
		Roles:      []string{u.Role},
		AuthMethod: MethodSession,
	}, nil
}

func (s *Service) identityFromAPIKey(ctx context.Context, presented string) (*Identity, error) {
	hash := HashAPIKey(presented)
	k, err := s.store.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNoRows) {
			return nil, ErrUnauthenticated
		}
		return nil, err
	}
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return nil, ErrUnauthenticated
	}
	u, err := s.store.GetUserByID(ctx, k.UserID)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	// best-effort touch; never fail the request because of it
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.store.TouchAPIKey(bg, k.ID); err != nil && s.logger != nil {
			s.logger.Debug("auth: touch api key failed", "err", err, "key_id", k.ID)
		}
	}()
	return &Identity{
		UserID:     u.ID,
		Username:   u.Username,
		Email:      u.Email,
		Roles:      []string{u.Role},
		AuthMethod: MethodAPIKey,
	}, nil
}

func (s *Service) upsertProxyUser(ctx context.Context, id *Identity) (User, error) {
	user, err := s.store.GetUserByUsername(ctx, id.Username)
	if err == nil {
		role := id.proxyDerivedRole()
		if user.Email != id.Email || (role != "" && user.Role != role) {
			updated, err := s.store.UpdateUserOIDC(ctx, user.ID, id.Email, role)
			if err == nil {
				return updated, nil
			}
		}
		return user, nil
	}
	if !errors.Is(err, ErrNoRows) {
		return User{}, err
	}
	pw, err := randomBase62(32)
	if err != nil {
		return User{}, err
	}
	hash, err := HashPassword(pw)
	if err != nil {
		return User{}, err
	}
	role := id.proxyDerivedRole()
	if role == "" {
		role = "user"
	}
	return s.store.CreateUser(ctx, CreateUserParams{
		Username:     id.Username,
		PasswordHash: hash,
		Email:        id.Email,
		Role:         role,
	})
}

func (i *Identity) proxyDerivedRole() string {
	if i.HasRole("admin") {
		return "admin"
	}
	return "user"
}

func extractAPIKey(r *http.Request) string {
	if k := strings.TrimSpace(r.Header.Get("X-Api-Key")); k != "" {
		return k
	}
	if a := strings.TrimSpace(r.Header.Get("Authorization")); a != "" {
		const p = "bearer "
		if len(a) > len(p) && strings.EqualFold(a[:len(p)], p) {
			return strings.TrimSpace(a[len(p):])
		}
	}
	return ""
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}
