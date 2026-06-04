package auth_test

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

const adminMetaKey = "auth.admin_user_id"

// newUserMgmtService builds a Service backed by a fresh sqlite DB plus a seeded
// admin user that is registered as the protected primary admin.
func newUserMgmtService(t *testing.T) (*auth.Service, auth.Store, int64) {
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
		Username:     "admin",
		PasswordHash: hash,
		Email:        "admin@example.com",
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := store.SetMeta(ctx, adminMetaKey, strconv.FormatInt(admin.ID, 10)); err != nil {
		t.Fatalf("set admin meta: %v", err)
	}
	svc, err := auth.NewService(auth.ServiceOptions{
		Store:         store,
		Logger:        quietLogger(),
		AppConfig:     &appconfig.Config{},
		AppConfigPath: filepath.Join(dir, "config.json"),
		SessionSecret: []byte("a-test-session-secret-32bytes!!!"),
		SessionTTL:    time.Hour,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, store, admin.ID
}

func TestCreateAndListUsers(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID := newUserMgmtService(t)

	u, err := svc.CreateUserAccount(ctx, "bob", "password1", "bob@example.com", "user")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.Role != "user" || u.Username != "bob" {
		t.Fatalf("unexpected user: %+v", u)
	}

	users, err := svc.ListUserAccounts(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	_ = adminID
}

func TestCreateUserValidation(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newUserMgmtService(t)

	if _, err := svc.CreateUserAccount(ctx, "", "password1", "", "user"); err != auth.ErrInvalidUsername {
		t.Fatalf("empty username: got %v", err)
	}
	if _, err := svc.CreateUserAccount(ctx, "x", "short", "", "user"); err != auth.ErrWeakPassword {
		t.Fatalf("weak password: got %v", err)
	}
	if _, err := svc.CreateUserAccount(ctx, "x", "password1", "", "superuser"); err != auth.ErrInvalidRole {
		t.Fatalf("bad role: got %v", err)
	}
	// duplicate
	if _, err := svc.CreateUserAccount(ctx, "dup", "password1", "", "user"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := svc.CreateUserAccount(ctx, "dup", "password1", "", "user"); err != auth.ErrUserExists {
		t.Fatalf("duplicate: got %v", err)
	}
}

func TestDeleteUserGuards(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID := newUserMgmtService(t)

	bob, err := svc.CreateUserAccount(ctx, "bob", "password1", "", "user")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Cannot delete the protected primary admin.
	if err := svc.DeleteUserAccount(ctx, bob.ID, adminID); err != auth.ErrProtectedUser {
		t.Fatalf("delete admin: got %v", err)
	}
	// Cannot delete self.
	if err := svc.DeleteUserAccount(ctx, bob.ID, bob.ID); err != auth.ErrSelfModify {
		t.Fatalf("delete self: got %v", err)
	}
	// Not found.
	if err := svc.DeleteUserAccount(ctx, adminID, 99999); err != auth.ErrUserNotFound {
		t.Fatalf("delete missing: got %v", err)
	}
	// Happy path: admin deletes bob.
	if err := svc.DeleteUserAccount(ctx, adminID, bob.ID); err != nil {
		t.Fatalf("delete bob: %v", err)
	}
	users, _ := svc.ListUserAccounts(ctx)
	if len(users) != 1 {
		t.Fatalf("expected 1 user after delete, got %d", len(users))
	}
}

func TestSetUserRoleGuards(t *testing.T) {
	ctx := context.Background()
	svc, _, adminID := newUserMgmtService(t)

	bob, err := svc.CreateUserAccount(ctx, "bob", "password1", "", "user")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := svc.SetUserRole(ctx, bob.ID, adminID, "user"); err != auth.ErrProtectedUser {
		t.Fatalf("demote admin: got %v", err)
	}
	if _, err := svc.SetUserRole(ctx, bob.ID, bob.ID, "admin"); err != auth.ErrSelfModify {
		t.Fatalf("self role: got %v", err)
	}
	if _, err := svc.SetUserRole(ctx, adminID, bob.ID, "wizard"); err != auth.ErrInvalidRole {
		t.Fatalf("bad role: got %v", err)
	}
	u, err := svc.SetUserRole(ctx, adminID, bob.ID, "admin")
	if err != nil {
		t.Fatalf("promote bob: %v", err)
	}
	if u.Role != "admin" {
		t.Fatalf("expected admin, got %s", u.Role)
	}
}

func TestResetUserPassword(t *testing.T) {
	ctx := context.Background()
	svc, store, adminID := newUserMgmtService(t)

	bob, err := svc.CreateUserAccount(ctx, "bob", "password1", "", "user")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.ResetUserPassword(ctx, bob.ID, "short"); err != auth.ErrWeakPassword {
		t.Fatalf("weak: got %v", err)
	}
	if err := svc.ResetUserPassword(ctx, adminID, "newpassword1"); err != auth.ErrProtectedUser {
		t.Fatalf("reset admin: got %v", err)
	}
	if err := svc.ResetUserPassword(ctx, bob.ID, "newpassword1"); err != nil {
		t.Fatalf("reset bob: %v", err)
	}
	updated, err := store.GetUserByID(ctx, bob.ID)
	if err != nil {
		t.Fatalf("get bob: %v", err)
	}
	ok, err := auth.VerifyPassword(updated.PasswordHash, "newpassword1")
	if err != nil || !ok {
		t.Fatalf("verify new password: ok=%v err=%v", ok, err)
	}
}
