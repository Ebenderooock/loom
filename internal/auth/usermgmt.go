package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// User-management errors surfaced to admin handlers.
var (
	ErrUserExists      = errors.New("auth: username already exists")
	ErrInvalidRole     = errors.New("auth: role must be 'admin' or 'user'")
	ErrInvalidUsername = errors.New("auth: username is required")
	ErrWeakPassword    = errors.New("auth: password must be at least 8 characters")
	ErrProtectedUser   = errors.New("auth: the primary admin account cannot be modified or deleted")
	ErrSelfModify      = errors.New("auth: you cannot delete or change the role of your own account")
	ErrUserNotFound    = errors.New("auth: user not found")
)

const minPasswordLen = 8

func validRole(role string) bool {
	return role == "admin" || role == "user"
}

// isUniqueViolation reports whether err is a duplicate-key error from either
// engine, used to map a racing concurrent create to ErrUserExists.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate key")
}

// primaryAdminID returns the config-managed admin user's DB id (0 if untracked).
// It fails closed: a present-but-unparseable id is treated as an error so the
// protected-admin guards never silently disable.
func (s *Service) primaryAdminID(ctx context.Context) (int64, error) {
	idStr, err := s.store.GetMeta(ctx, schemaMetaAdminUserID)
	if err != nil {
		return 0, err
	}
	if idStr == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("auth: invalid admin user id in schema_meta: %w", err)
	}
	return id, nil
}

// ListUserAccounts returns all users for admin display.
func (s *Service) ListUserAccounts(ctx context.Context) ([]UserSummary, error) {
	return s.store.ListUsers(ctx)
}

// CreateUserAccount provisions a new DB-backed user with the given role.
func (s *Service) CreateUserAccount(ctx context.Context, username, password, email, role string) (User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	if role == "" {
		role = "user"
	}
	if username == "" {
		return User{}, ErrInvalidUsername
	}
	if !validRole(role) {
		return User{}, ErrInvalidRole
	}
	if len(password) < minPasswordLen {
		return User{}, ErrWeakPassword
	}

	if _, err := s.store.GetUserByUsername(ctx, username); err == nil {
		return User{}, ErrUserExists
	} else if !errors.Is(err, ErrNoRows) {
		return User{}, err
	}

	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	u, err := s.store.CreateUser(ctx, CreateUserParams{
		Username:     username,
		PasswordHash: hash,
		Email:        email,
		Role:         role,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrUserExists
		}
		return User{}, err
	}
	s.logger.Info("user account created", "username", u.Username, "id", u.ID, "role", role)
	return u, nil
}

// DeleteUserAccount removes a user, guarding the primary admin and self.
func (s *Service) DeleteUserAccount(ctx context.Context, actorID, targetID int64) error {
	if actorID == targetID {
		return ErrSelfModify
	}
	adminID, err := s.primaryAdminID(ctx)
	if err != nil {
		return err
	}
	if adminID != 0 && targetID == adminID {
		return ErrProtectedUser
	}
	if _, err := s.store.GetUserByID(ctx, targetID); err != nil {
		if errors.Is(err, ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}
	if err := s.store.DeleteUser(ctx, targetID); err != nil {
		return err
	}
	s.logger.Info("user account deleted", "id", targetID, "actor", actorID)
	return nil
}

// SetUserRole changes a user's role, guarding the primary admin and self.
func (s *Service) SetUserRole(ctx context.Context, actorID, targetID int64, role string) (User, error) {
	if !validRole(role) {
		return User{}, ErrInvalidRole
	}
	if actorID == targetID {
		return User{}, ErrSelfModify
	}
	adminID, err := s.primaryAdminID(ctx)
	if err != nil {
		return User{}, err
	}
	if adminID != 0 && targetID == adminID {
		return User{}, ErrProtectedUser
	}
	if _, err := s.store.GetUserByID(ctx, targetID); err != nil {
		if errors.Is(err, ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	if err := s.store.UpdateUserRole(ctx, targetID, role); err != nil {
		return User{}, err
	}
	u, err := s.store.GetUserByID(ctx, targetID)
	if err != nil {
		return User{}, err
	}
	s.logger.Info("user role changed", "id", targetID, "role", role, "actor", actorID)
	return u, nil
}

// ResetUserPassword sets a new password for a user. The primary admin's
// password is config-managed and cannot be reset here.
func (s *Service) ResetUserPassword(ctx context.Context, targetID int64, password string) error {
	if len(password) < minPasswordLen {
		return ErrWeakPassword
	}
	adminID, err := s.primaryAdminID(ctx)
	if err != nil {
		return err
	}
	if adminID != 0 && targetID == adminID {
		return ErrProtectedUser
	}
	if _, err := s.store.GetUserByID(ctx, targetID); err != nil {
		if errors.Is(err, ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	if err := s.store.UpdateUserPassword(ctx, targetID, hash); err != nil {
		return err
	}
	s.logger.Info("user password reset", "id", targetID)
	return nil
}
