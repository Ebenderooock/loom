package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Invite-related errors surfaced to handlers.
var (
	ErrInvitesDisabled = errors.New("auth: invites are not enabled")
	ErrInviteInvalid   = errors.New("auth: invite is invalid, expired, or already used")
)

// Invite expiry bounds. Admin-supplied durations are clamped to this range so a
// mistake can't create an immediately-expired or effectively-permanent link.
const (
	defaultInviteTTL = 7 * 24 * time.Hour
	minInviteTTL     = time.Hour
	maxInviteTTL     = 90 * 24 * time.Hour
)

// InviteStatus is the derived lifecycle state of an invite.
type InviteStatus string

const (
	InvitePending InviteStatus = "pending"
	InviteUsed    InviteStatus = "used"
	InviteExpired InviteStatus = "expired"
)

// status derives an invite's lifecycle state at time now.
func inviteStatus(inv Invite, now time.Time) InviteStatus {
	switch {
	case !inv.UsedAt.IsZero():
		return InviteUsed
	case !inv.ExpiresAt.IsZero() && !inv.ExpiresAt.After(now):
		return InviteExpired
	default:
		return InvitePending
	}
}

// generateInviteToken returns a URL-safe, high-entropy (256-bit) token.
func generateInviteToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateInvite provisions a single-use invite with a pre-assigned role. ttl is
// clamped to [minInviteTTL, maxInviteTTL]; a non-positive ttl uses the default.
func (s *Service) CreateInvite(ctx context.Context, actorID int64, email, role string, ttl time.Duration) (Invite, error) {
	if s.invites == nil {
		return Invite{}, ErrInvitesDisabled
	}
	email = strings.TrimSpace(email)
	if role == "" {
		role = "user"
	}
	if !validRole(role) {
		return Invite{}, ErrInvalidRole
	}
	switch {
	case ttl <= 0:
		ttl = defaultInviteTTL
	case ttl < minInviteTTL:
		ttl = minInviteTTL
	case ttl > maxInviteTTL:
		ttl = maxInviteTTL
	}
	token, err := generateInviteToken()
	if err != nil {
		return Invite{}, err
	}
	now := time.Now().UTC()
	inv := Invite{
		ID:        uuid.NewString(),
		Token:     token,
		Email:     email,
		Role:      role,
		CreatedBy: actorID,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if err := s.invites.Create(ctx, inv); err != nil {
		return Invite{}, err
	}
	s.logger.Info("invite created", "id", inv.ID, "role", role, "actor", actorID, "expires_at", inv.ExpiresAt)
	return inv, nil
}

// ListInvites returns all invites for admin display.
func (s *Service) ListInvites(ctx context.Context) ([]Invite, error) {
	if s.invites == nil {
		return nil, ErrInvitesDisabled
	}
	return s.invites.List(ctx)
}

// InviteStatusAt exposes the derived status helper to handlers.
func (s *Service) InviteStatusAt(inv Invite, now time.Time) InviteStatus {
	return inviteStatus(inv, now)
}

// RevokeInvite deletes an invite. Revoking an already-redeemed invite removes
// only the audit row; the provisioned account is unaffected.
func (s *Service) RevokeInvite(ctx context.Context, id string) error {
	if s.invites == nil {
		return ErrInvitesDisabled
	}
	ok, err := s.invites.Delete(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInviteNotFound
	}
	s.logger.Info("invite revoked", "id", id)
	return nil
}

// LookupInvite returns the invite for token only if it is currently redeemable,
// otherwise ErrInviteInvalid (callers must not distinguish the reason to avoid
// leaking which tokens ever existed).
func (s *Service) LookupInvite(ctx context.Context, token string) (Invite, error) {
	if s.invites == nil {
		return Invite{}, ErrInvitesDisabled
	}
	inv, err := s.invites.GetByToken(ctx, token)
	if err != nil {
		if errors.Is(err, ErrInviteNotFound) {
			return Invite{}, ErrInviteInvalid
		}
		return Invite{}, err
	}
	if inviteStatus(inv, time.Now().UTC()) != InvitePending {
		return Invite{}, ErrInviteInvalid
	}
	return inv, nil
}

// AcceptInvite redeems a pending invite, creating a new account with the
// invite's pre-assigned role and email. The flow is: pre-validate the supplied
// credentials (so a recoverable signup error doesn't burn the link), atomically
// claim the invite (guarding against concurrent or expired redemption), create
// the account, then either finalize the claim with the new user's id or release
// the claim on failure so the link can be retried.
func (s *Service) AcceptInvite(ctx context.Context, token, username, password string) (User, error) {
	if s.invites == nil {
		return User{}, ErrInvitesDisabled
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return User{}, ErrInvalidUsername
	}
	if len(password) < minPasswordLen {
		return User{}, ErrWeakPassword
	}

	inv, err := s.invites.GetByToken(ctx, token)
	if err != nil {
		if errors.Is(err, ErrInviteNotFound) {
			return User{}, ErrInviteInvalid
		}
		return User{}, err
	}
	if inviteStatus(inv, time.Now().UTC()) != InvitePending {
		return User{}, ErrInviteInvalid
	}
	// Reject an already-taken username before consuming the invite.
	if _, err := s.store.GetUserByUsername(ctx, username); err == nil {
		return User{}, ErrUserExists
	} else if !errors.Is(err, ErrNoRows) {
		return User{}, err
	}

	claimed, err := s.invites.Claim(ctx, token, time.Now().UTC())
	if err != nil {
		return User{}, err
	}
	if !claimed {
		// Lost a race or it expired between the read and the claim.
		return User{}, ErrInviteInvalid
	}

	u, err := s.CreateUserAccount(ctx, username, password, inv.Email, inv.Role)
	if err != nil {
		if relErr := s.invites.Release(ctx, token); relErr != nil {
			s.logger.Error("failed to release invite after signup error", "token_id", inv.ID, "err", relErr)
		}
		return User{}, err
	}
	if finErr := s.invites.Finalize(ctx, token, u.ID); finErr != nil {
		// The account exists and the invite is consumed; only the audit link is
		// missing. Log rather than fail the redemption.
		s.logger.Error("failed to finalize invite used_by", "token_id", inv.ID, "user_id", u.ID, "err", finErr)
	}
	s.logger.Info("invite redeemed", "id", inv.ID, "user_id", u.ID, "role", inv.Role)
	return u, nil
}
