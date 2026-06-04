package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type userSummaryOut struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email,omitempty"`
	Role      string `json:"role"`
	Protected bool   `json:"protected"`
	CreatedAt string `json:"created_at,omitempty"`
}

func (s *Service) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.ListUserAccounts(r.Context())
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "list users")
		return
	}
	adminID, _ := s.primaryAdminID(r.Context())
	out := make([]userSummaryOut, len(users))
	for i, u := range users {
		o := userSummaryOut{
			ID:        u.ID,
			Username:  u.Username,
			Email:     u.Email,
			Role:      u.Role,
			Protected: adminID != 0 && u.ID == adminID,
		}
		if !u.CreatedAt.IsZero() {
			o.CreatedAt = u.CreatedAt.UTC().Format(time.RFC3339)
		}
		out[i] = o
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{"users": out})
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func (s *Service) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := s.CreateUserAccount(r.Context(), req.Username, req.Password, req.Email, req.Role)
	if err != nil {
		writeUserMgmtError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, map[string]any{"user": toUserOut(u)})
}

func (s *Service) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	if id == nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	targetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := s.DeleteUserAccount(r.Context(), id.UserID, targetID); err != nil {
		writeUserMgmtError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

func (s *Service) handleUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	if id == nil {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	targetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req updateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := s.SetUserRole(r.Context(), id.UserID, targetID, req.Role)
	if err != nil {
		writeUserMgmtError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{"user": toUserOut(u)})
}

type resetPasswordRequest struct {
	Password string `json:"password"`
}

func (s *Service) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	targetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.ResetUserPassword(r.Context(), targetID, req.Password); err != nil {
		writeUserMgmtError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeUserMgmtError maps service errors to appropriate HTTP status codes.
func writeUserMgmtError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUserExists):
		writeAuthError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidRole), errors.Is(err, ErrInvalidUsername), errors.Is(err, ErrWeakPassword):
		writeAuthError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrProtectedUser), errors.Is(err, ErrSelfModify):
		writeAuthError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrUserNotFound):
		writeAuthError(w, http.StatusNotFound, err.Error())
	default:
		writeAuthError(w, http.StatusInternalServerError, "internal error")
	}
}
