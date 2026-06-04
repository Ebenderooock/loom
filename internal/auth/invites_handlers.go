package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type inviteOut struct {
	ID         string `json:"id"`
	Token      string `json:"token,omitempty"`
	URL        string `json:"url,omitempty"`
	Email      string `json:"email,omitempty"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	UsedAt     string `json:"used_at,omitempty"`
	UsedByName string `json:"used_by_name,omitempty"`
}

// toInviteOut renders an invite for the admin list. The raw token (and thus the
// shareable URL) is only exposed while the invite is still pending so a consumed
// or expired link can't be re-shared.
func (s *Service) toInviteOut(inv Invite, r *http.Request, now time.Time) inviteOut {
	status := inviteStatus(inv, now)
	o := inviteOut{
		ID:     inv.ID,
		Email:  inv.Email,
		Role:   inv.Role,
		Status: string(status),
	}
	if !inv.CreatedAt.IsZero() {
		o.CreatedAt = inv.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !inv.ExpiresAt.IsZero() {
		o.ExpiresAt = inv.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if !inv.UsedAt.IsZero() {
		o.UsedAt = inv.UsedAt.UTC().Format(time.RFC3339)
	}
	o.UsedByName = inv.UsedByName
	if status == InvitePending {
		o.Token = inv.Token
		o.URL = inviteURL(r, inv.Token)
	}
	return o
}

// inviteURL builds the public redemption link from the request's origin.
func inviteURL(r *http.Request, token string) string {
	scheme := "http"
	if requestIsTLS(r) || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host + "/invite/" + token
}

type createInviteRequest struct {
	Email          string `json:"email"`
	Role           string `json:"role"`
	ExpiresInHours int    `json:"expires_in_hours"`
}

func (s *Service) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := IdentityFrom(r.Context())
	if id == nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid json")
		return
	}
	var ttl time.Duration
	if req.ExpiresInHours > 0 {
		ttl = time.Duration(req.ExpiresInHours) * time.Hour
	}
	inv, err := s.CreateInvite(r.Context(), id.UserID, req.Email, req.Role, ttl)
	if err != nil {
		writeInviteError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, map[string]any{"invite": s.toInviteOut(inv, r, time.Now().UTC())})
}

func (s *Service) handleListInvites(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	invs, err := s.ListInvites(r.Context())
	if err != nil {
		writeInviteError(w, err)
		return
	}
	now := time.Now().UTC()
	out := make([]inviteOut, len(invs))
	for i, inv := range invs {
		out[i] = s.toInviteOut(inv, r, now)
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{"invites": out})
}

func (s *Service) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.RevokeInvite(r.Context(), id); err != nil {
		writeInviteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type publicInviteOut struct {
	Valid bool   `json:"valid"`
	Email string `json:"email,omitempty"`
	Role  string `json:"role,omitempty"`
}

func (s *Service) handlePublicInvite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	token := chi.URLParam(r, "token")
	inv, err := s.LookupInvite(r.Context(), token)
	if err != nil {
		if errors.Is(err, ErrInviteInvalid) {
			writeJSONStatus(w, http.StatusOK, publicInviteOut{Valid: false})
			return
		}
		writeInviteError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusOK, publicInviteOut{Valid: true, Email: inv.Email, Role: inv.Role})
}

type acceptInviteRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Service) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	token := chi.URLParam(r, "token")
	var req acceptInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := s.AcceptInvite(r.Context(), token, req.Username, req.Password)
	if err != nil {
		writeInviteError(w, err)
		return
	}
	if err := IssueSessionCookie(w, r, s.sessionSecret, u.ID, s.sessionTTL, s.cookieSecure); err != nil {
		writeAuthError(w, http.StatusInternalServerError, "issue session")
		return
	}
	writeJSONStatus(w, http.StatusCreated, toUserOut(u))
}

// writeInviteError maps invite/user-management errors to HTTP status codes.
func writeInviteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvitesDisabled):
		writeAuthError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ErrInviteInvalid):
		writeAuthError(w, http.StatusGone, err.Error())
	case errors.Is(err, ErrInviteNotFound):
		writeAuthError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrUserExists):
		writeAuthError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidRole), errors.Is(err, ErrInvalidUsername), errors.Is(err, ErrWeakPassword):
		writeAuthError(w, http.StatusBadRequest, err.Error())
	default:
		writeAuthError(w, http.StatusInternalServerError, "invite operation failed")
	}
}
