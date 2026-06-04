package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Mount attaches all /api/v1/auth/* routes to r. The /me handler is
// gated by RequireAuth; the rest are public.
func (s *Service) Mount(r chi.Router) {
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Get("/status", s.handleStatus)
		r.Post("/initialize", s.handleInitialize)
		r.Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)
		r.Get("/oidc/login", s.handleOIDCLogin)
		r.Get("/oidc/callback", s.handleOIDCCallback)
		// Public invite redemption (token holder self-registers).
		r.Get("/invites/redeem/{token}", s.handlePublicInvite)
		r.Post("/invites/redeem/{token}/accept", s.handleAcceptInvite)
		r.Group(func(r chi.Router) {
			r.Use(s.RequireAuth)
			r.Get("/me", s.handleMe)
			r.Post("/apikeys", s.handleCreateAPIKey)
			r.Get("/apikeys", s.handleListAPIKeys)
			r.Delete("/apikeys/{id}", s.handleRevokeAPIKey)
		})
		r.Group(func(r chi.Router) {
			r.Use(s.RequireRole("admin"))
			r.Get("/users", s.handleListUsers)
			r.Post("/users", s.handleCreateUser)
			r.Delete("/users/{id}", s.handleDeleteUser)
			r.Patch("/users/{id}/role", s.handleUpdateUserRole)
			r.Post("/users/{id}/password", s.handleResetUserPassword)
			r.Post("/invites", s.handleCreateInvite)
			r.Get("/invites", s.handleListInvites)
			r.Delete("/invites/{id}", s.handleRevokeInvite)
		})
	})
}

type statusResponse struct {
	SetupRequired  bool    `json:"setup_required"`
	IsAuthenticated bool    `json:"is_authenticated"`
	User           *userOut `json:"user,omitempty"`
}

func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Setup is required ONLY if config.setup_complete is false.
	// The config flag is the source of truth.
	setupRequired := !s.appConfig.SetupComplete

	resp := statusResponse{
		SetupRequired: setupRequired,
	}

	// Check if user is authenticated (resolve directly since this route
	// is not behind RequireAuth middleware)
	id, _ := s.resolveIdentity(r)
	if id != nil {
		resp.IsAuthenticated = true
		out := userOut{
			ID:       id.UserID,
			Username: id.Username,
			Email:    id.Email,
			Role:     primaryRole(id.Roles),
		}
		resp.User = &out
	}

	writeJSONStatus(w, http.StatusOK, resp)
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

type initializeRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

type initializeResponse struct {
	User   userOut `json:"user"`
	APIKey string  `json:"api_key"`
}

// handleInitialize sets up initial admin credentials and writes them to app config.
// Only callable when config.setup_complete is false.
// The config flag is the authoritative source of truth for setup status.
func (s *Service) handleInitialize(w http.ResponseWriter, r *http.Request) {
	// Setup is blocked ONLY if config says it's complete
	if s.appConfig.SetupComplete {
		writeAuthError(w, http.StatusForbidden, "setup already complete")
		return
	}

	var req initializeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeAuthError(w, http.StatusBadRequest, "username and password required")
		return
	}

	// Hash password
	hash, err := HashPassword(req.Password)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "hash password")
		return
	}

	// Save to config FIRST (config is durable source of truth)
	s.appConfig.SetupComplete = true
	s.appConfig.Admin.Username = req.Username
	s.appConfig.Admin.PasswordHash = hash
	if err := s.appConfig.Save(s.appConfigPath); err != nil {
		// Revert in-memory state
		s.appConfig.SetupComplete = false
		s.appConfig.Admin.Username = ""
		s.appConfig.Admin.PasswordHash = ""
		s.logger.Error("failed to save app config", "err", err)
		writeAuthError(w, http.StatusInternalServerError, "save config")
		return
	}

	// Reconcile admin user in database (upsert: preserves user ID and API keys)
	u, err := s.ReconcileAdmin(r.Context())
	if err != nil {
		s.logger.Error("failed to reconcile admin user", "err", err)
		writeAuthError(w, http.StatusInternalServerError, "create user")
		return
	}

	// Generate API key for integrations (only if user has none)
	keys, err := s.store.ListAPIKeysForUser(r.Context(), u.ID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "list api keys")
		return
	}

	var apiKey string
	if len(keys) == 0 {
		key, keyHash, prefix, err := GenerateAPIKey()
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "generate api key")
			return
		}
		_, err = s.store.CreateAPIKey(r.Context(), CreateAPIKeyParams{
			UserID:  u.ID,
			Name:    "Default",
			KeyHash: keyHash,
			Prefix:  prefix,
		})
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "create api key")
			return
		}
		apiKey = key
	} else {
		apiKey = keys[0].Prefix + "..."
	}

	// Issue session cookie
	if err := IssueSessionCookie(w, r, s.sessionSecret, u.ID, s.sessionTTL, s.cookieSecure); err != nil {
		writeAuthError(w, http.StatusInternalServerError, "issue session")
		return
	}

	writeJSONStatus(w, http.StatusCreated, initializeResponse{
		User:   toUserOut(u),
		APIKey: apiKey,
	})
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

type createAPIKeyRequest struct {
	Name string `json:"name"`
}

type apiKeyOut struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	CreatedAt string `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	ExpiresAt *string `json:"expires_at,omitempty"`
}

func toAPIKeyOut(ak APIKey) apiKeyOut {
	out := apiKeyOut{
		ID:        ak.ID,
		Name:      ak.Name,
		Prefix:    ak.Prefix,
		CreatedAt: ak.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if ak.LastUsedAt != nil {
		s := ak.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
		out.LastUsedAt = &s
	}
	if ak.ExpiresAt != nil {
		s := ak.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
		out.ExpiresAt = &s
	}
	return out
}

func (s *Service) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	if id == nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid json")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeAuthError(w, http.StatusBadRequest, "name required")
		return
	}

	// Generate a new API key
	key, keyHash, prefix, err := GenerateAPIKey()
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "generate api key")
		return
	}

	ak, err := s.store.CreateAPIKey(r.Context(), CreateAPIKeyParams{
		UserID:  id.UserID,
		Name:    req.Name,
		KeyHash: keyHash,
		Prefix:  prefix,
	})
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "create api key")
		return
	}

	out := struct {
		apiKeyOut
		Key string `json:"key"`
	}{
		apiKeyOut: toAPIKeyOut(ak),
		Key:       key,
	}
	writeJSONStatus(w, http.StatusCreated, out)
}

func (s *Service) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	if id == nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	aks, err := s.store.ListAPIKeysForUser(r.Context(), id.UserID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "list api keys")
		return
	}

	out := make([]apiKeyOut, len(aks))
	for i, ak := range aks {
		out[i] = toAPIKeyOut(ak)
	}

	writeJSONStatus(w, http.StatusOK, map[string]any{
		"keys": out,
	})
}

func (s *Service) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	if id == nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	keyID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := s.store.RevokeAPIKey(r.Context(), keyID, id.UserID); err != nil {
		writeAuthError(w, http.StatusInternalServerError, "revoke api key")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
