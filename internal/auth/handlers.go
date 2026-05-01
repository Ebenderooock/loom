package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Mount attaches all /api/v1/auth/* routes to r. The /me handler is
// gated by RequireAuth; the rest are public.
func (s *Service) Mount(r chi.Router) {
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/setup", s.handleSetup)
		r.Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)
		r.Get("/oidc/login", s.handleOIDCLogin)
		r.Get("/oidc/callback", s.handleOIDCCallback)
		r.Group(func(r chi.Router) {
			r.Use(s.RequireAuth)
			r.Get("/me", s.handleMe)
		})
	})
}

type userOut struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Role     string `json:"role"`
}

func toUserOut(u User) userOut {
	return userOut{ID: u.ID, Username: u.Username, Email: u.Email, Role: u.Role}
}

type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

func (s *Service) handleSetup(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.CountUsers(r.Context())
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if count > 0 {
		writeAuthError(w, http.StatusForbidden, "setup already complete")
		return
	}
	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeAuthError(w, http.StatusBadRequest, "username and password required")
		return
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "hash password")
		return
	}
	u, err := s.store.CreateUser(r.Context(), CreateUserParams{
		Username:     req.Username,
		PasswordHash: hash,
		Email:        strings.TrimSpace(req.Email),
		Role:         "admin",
	})
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "create user")
		return
	}
	if err := IssueSessionCookie(w, r, s.sessionSecret, u.ID, s.sessionTTL, s.cookieSecure); err != nil {
		writeAuthError(w, http.StatusInternalServerError, "issue session")
		return
	}
	writeJSONStatus(w, http.StatusCreated, toUserOut(u))
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeAuthError(w, http.StatusBadRequest, "username and password required")
		return
	}
	u, err := s.store.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	ok, err := VerifyPassword(u.PasswordHash, req.Password)
	if err != nil || !ok {
		writeAuthError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := IssueSessionCookie(w, r, s.sessionSecret, u.ID, s.sessionTTL, s.cookieSecure); err != nil {
		writeAuthError(w, http.StatusInternalServerError, "issue session")
		return
	}
	writeJSONStatus(w, http.StatusOK, toUserOut(u))
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	ClearSessionCookie(w, r, s.cookieSecure)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	if id == nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out := struct {
		userOut
		AuthMethod Method `json:"auth_method"`
	}{
		userOut: userOut{
			ID:       id.UserID,
			Username: id.Username,
			Email:    id.Email,
			Role:     primaryRole(id.Roles),
		},
		AuthMethod: id.AuthMethod,
	}
	writeJSONStatus(w, http.StatusOK, out)
}

func (s *Service) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	if !s.OIDCConfigured() {
		writeAuthError(w, http.StatusNotFound, "oidc not configured")
		return
	}
	state, cookieValue, err := SignOIDCState(s.sessionSecret)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "issue state")
		return
	}
	IssueOIDCStateCookie(w, r, cookieValue, s.cookieSecure)
	url, err := s.oidc.AuthCodeURL(r.Context(), state)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "oidc init")
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Service) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if !s.OIDCConfigured() {
		writeAuthError(w, http.StatusNotFound, "oidc not configured")
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		writeAuthError(w, http.StatusBadRequest, "missing state or code")
		return
	}
	c, err := r.Cookie(oidcStateCookie)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "missing state cookie")
		return
	}
	if err := VerifyOIDCState(s.sessionSecret, state, c.Value); err != nil {
		writeAuthError(w, http.StatusBadRequest, "state verification failed")
		return
	}
	ClearOIDCStateCookie(w, r, s.cookieSecure)
	tok, err := s.oidc.Exchange(r.Context(), code)
	if err != nil {
		writeAuthError(w, http.StatusBadGateway, "oidc exchange failed")
		return
	}
	claims, err := s.oidc.VerifyIDToken(r.Context(), tok)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "id token invalid")
		return
	}
	username := s.oidc.ClaimUsername(claims)
	email := s.oidc.ClaimEmail(claims)
	role := s.oidc.ClaimRole(claims)
	user, err := UpsertOIDCUser(r.Context(), s.store, username, email, role)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "upsert user")
		return
	}
	if err := IssueSessionCookie(w, r, s.sessionSecret, user.ID, s.sessionTTL, s.cookieSecure); err != nil {
		writeAuthError(w, http.StatusInternalServerError, "issue session")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func primaryRole(roles []string) string {
	for _, r := range roles {
		if r == "admin" {
			return "admin"
		}
	}
	if len(roles) > 0 {
		return roles[0]
	}
	return "user"
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Compile-time guard that errors.Is is referenced (lint).
var _ = errors.Is
