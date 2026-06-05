package auth_test

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

// newInviteService builds a Service with the invite store wired, returning the
// service, the raw invite store (for inserting expired fixtures and inspecting
// state), and the seeded primary admin id.
func newInviteService(t *testing.T) (*auth.Service, *auth.InviteStore, int64) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(dir, "auth.db")},
	}
	db, err := storage.Open(ctx, cfg, quietLogger())
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store, err := auth.StoreFromDB(db)
	if err != nil {
		t.Fatalf("StoreFromDB: %v", err)
	}
	hash, err := auth.HashPassword("hunter2!!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	admin, err := store.CreateUser(ctx, auth.CreateUserParams{
		Username: "admin", PasswordHash: hash, Email: "admin@example.com", Role: "admin",
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := store.SetMeta(ctx, adminMetaKey, strconv.FormatInt(admin.ID, 10)); err != nil {
		t.Fatalf("set admin meta: %v", err)
	}
	invites := auth.NewInviteStore(db.DB())
	svc, err := auth.NewService(auth.ServiceOptions{
		Store:         store,
		Logger:        quietLogger(),
		AppConfig:     &appconfig.Config{},
		AppConfigPath: filepath.Join(dir, "config.json"),
		SessionSecret: []byte("a-test-session-secret-32bytes!!!"),
		SessionTTL:    time.Hour,
		Invites:       invites,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, invites, admin.ID
}

func TestCreateAndListInvites(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID := newInviteService(t)

	inv, err := svc.CreateInvite(ctx, adminID, "friend@example.com", "user", 0)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if inv.Token == "" {
		t.Fatal("expected a generated token")
	}
	if inv.Role != "user" || inv.Email != "friend@example.com" {
		t.Fatalf("unexpected invite fields: %+v", inv)
	}
	if !inv.ExpiresAt.After(time.Now()) {
		t.Fatal("expected a future expiry")
	}

	list, err := svc.ListInvites(ctx)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 invite, got %d", len(list))
	}
	if got := svc.InviteStatusAt(list[0], time.Now()); got != auth.InvitePending {
		t.Fatalf("expected pending, got %s", got)
	}
}

func TestCreateInviteClampsTTLAndValidatesRole(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID := newInviteService(t)

	// Below the minimum is clamped up to at least an hour out.
	inv, err := svc.CreateInvite(ctx, adminID, "", "", time.Minute)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if inv.Role != "user" {
		t.Fatalf("expected default role user, got %s", inv.Role)
	}
	if time.Until(inv.ExpiresAt) < 59*time.Minute {
		t.Fatalf("expected ttl clamped to >= 1h, got %s", time.Until(inv.ExpiresAt))
	}

	if _, err := svc.CreateInvite(ctx, adminID, "", "superuser", 0); err == nil {
		t.Fatal("expected ErrInvalidRole for bad role")
	}
}

func TestAcceptInviteHappyPath(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID := newInviteService(t)

	inv, err := svc.CreateInvite(ctx, adminID, "friend@example.com", "user", 0)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	u, err := svc.AcceptInvite(ctx, inv.Token, "friend", "s3cretpw!")
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if u.Username != "friend" || u.Role != "user" || u.Email != "friend@example.com" {
		t.Fatalf("unexpected created user: %+v", u)
	}

	// Single-use: a second redemption must fail and not create another account.
	if _, err := svc.AcceptInvite(ctx, inv.Token, "friend2", "s3cretpw!"); !errors.Is(err, auth.ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid on reuse, got %v", err)
	}

	list, _ := svc.ListInvites(ctx)
	if got := svc.InviteStatusAt(list[0], time.Now()); got != auth.InviteUsed {
		t.Fatalf("expected used status, got %s", got)
	}
	if list[0].UsedByName != "friend" {
		t.Fatalf("expected used_by_name 'friend', got %q", list[0].UsedByName)
	}
}

func TestAcceptInviteExpired(t *testing.T) {
	ctx := context.Background()
	svc, store, adminID := newInviteService(t)

	// Insert an already-expired invite directly.
	past := time.Now().Add(-2 * time.Hour)
	if err := store.Create(ctx, auth.Invite{
		ID: "expired-1", Token: "expiredtoken", Role: "user",
		CreatedBy: adminID, CreatedAt: past.Add(-time.Hour), ExpiresAt: past,
	}); err != nil {
		t.Fatalf("seed expired invite: %v", err)
	}
	if _, err := svc.LookupInvite(ctx, "expiredtoken"); !errors.Is(err, auth.ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid on lookup, got %v", err)
	}
	if _, err := svc.AcceptInvite(ctx, "expiredtoken", "late", "s3cretpw!"); !errors.Is(err, auth.ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid on accept, got %v", err)
	}
}

func TestAcceptInviteValidationDoesNotBurnLink(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID := newInviteService(t)

	inv, err := svc.CreateInvite(ctx, adminID, "", "user", 0)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	// Weak password is rejected before the invite is claimed.
	if _, err := svc.AcceptInvite(ctx, inv.Token, "bob", "short"); !errors.Is(err, auth.ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
	// Empty username likewise.
	if _, err := svc.AcceptInvite(ctx, inv.Token, "  ", "s3cretpw!"); !errors.Is(err, auth.ErrInvalidUsername) {
		t.Fatalf("expected ErrInvalidUsername, got %v", err)
	}
	// The invite is still redeemable.
	if _, err := svc.AcceptInvite(ctx, inv.Token, "bob", "s3cretpw!"); err != nil {
		t.Fatalf("expected redeemable invite after validation errors, got %v", err)
	}
}

func TestAcceptInviteDuplicateUsernameReleasesLink(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID := newInviteService(t)

	inv, err := svc.CreateInvite(ctx, adminID, "", "user", 0)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	// "admin" already exists.
	if _, err := svc.AcceptInvite(ctx, inv.Token, "admin", "s3cretpw!"); !errors.Is(err, auth.ErrUserExists) {
		t.Fatalf("expected ErrUserExists, got %v", err)
	}
	list, _ := svc.ListInvites(ctx)
	if got := svc.InviteStatusAt(list[0], time.Now()); got != auth.InvitePending {
		t.Fatalf("expected invite still pending after duplicate-username failure, got %s", got)
	}
	// And it can be redeemed with a fresh username.
	if _, err := svc.AcceptInvite(ctx, inv.Token, "newperson", "s3cretpw!"); err != nil {
		t.Fatalf("expected successful redemption, got %v", err)
	}
}

func TestRevokeInvite(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID := newInviteService(t)

	inv, err := svc.CreateInvite(ctx, adminID, "", "user", 0)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if err := svc.RevokeInvite(ctx, inv.ID); err != nil {
		t.Fatalf("RevokeInvite: %v", err)
	}
	if _, err := svc.LookupInvite(ctx, inv.Token); !errors.Is(err, auth.ErrInviteInvalid) {
		t.Fatalf("expected revoked invite to be invalid, got %v", err)
	}
	if err := svc.RevokeInvite(ctx, inv.ID); !errors.Is(err, auth.ErrInviteNotFound) {
		t.Fatalf("expected ErrInviteNotFound on second revoke, got %v", err)
	}
}

func TestInvitesDisabledWhenStoreNil(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	svc, err := auth.NewService(auth.ServiceOptions{
		Store:         stubAuthStore(t, dir),
		Logger:        quietLogger(),
		AppConfig:     &appconfig.Config{},
		AppConfigPath: filepath.Join(dir, "config.json"),
		SessionSecret: []byte("a-test-session-secret-32bytes!!!"),
		SessionTTL:    time.Hour,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, err := svc.CreateInvite(ctx, 1, "", "user", 0); !errors.Is(err, auth.ErrInvitesDisabled) {
		t.Fatalf("expected ErrInvitesDisabled, got %v", err)
	}
	if _, err := svc.LookupInvite(ctx, "x"); !errors.Is(err, auth.ErrInvitesDisabled) {
		t.Fatalf("expected ErrInvitesDisabled, got %v", err)
	}
}

// stubAuthStore opens a migrated sqlite store for the invites-disabled test.
func stubAuthStore(t *testing.T, dir string) auth.Store {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(dir, "stub.db")},
	}, quietLogger())
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store, err := auth.StoreFromDB(db)
	if err != nil {
		t.Fatalf("StoreFromDB: %v", err)
	}
	return store
}
