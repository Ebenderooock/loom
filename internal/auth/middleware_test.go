package auth_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomctl/loom/internal/auth"
	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/storage"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestService(t *testing.T) (*auth.Service, auth.Store, int64) {
	t.Helper()
	ctx := context.Background()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "auth.db")},
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
	hash, err := auth.HashPassword("hunter2!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u, err := store.CreateUser(ctx, auth.CreateUserParams{
		Username:     "alice",
		PasswordHash: hash,
		Email:        "alice@example.com",
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	proxy := auth.NewProxyAuth(true, []string{"127.0.0.0/8"}, "Remote-User", "Remote-Email", "Remote-Groups", []string{"admins"})
	svc, err := auth.NewService(auth.ServiceOptions{
		Store:         store,
		Logger:        quietLogger(),
		SessionSecret: []byte("a-test-session-secret-32bytes!!!"),
		SessionTTL:    time.Hour,
		Proxy:         proxy,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, store, u.ID
}

func protected(svc *auth.Service) http.Handler {
	return svc.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := auth.IdentityFrom(r.Context())
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uid":    id.UserID,
			"method": id.AuthMethod,
		})
	}))
}

func TestRequireAuthNoCreds(t *testing.T) {
	t.Parallel()
	svc, _, _ := newTestService(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	protected(svc).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401 got %d", rr.Code)
	}
}

func TestRequireAuthValidSession(t *testing.T) {
	t.Parallel()
	svc, _, uid := newTestService(t)
	now := time.Now()
	tok, err := auth.SignSession(svc.SessionSecret(), auth.SessionPayload{
		UID: uid, IAT: now.Unix(), EXP: now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: tok})
	protected(svc).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200 got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"method":"session"`) {
		t.Fatalf("expected session method, got %s", rr.Body.String())
	}
}

func TestRequireAuthTamperedSession(t *testing.T) {
	t.Parallel()
	svc, _, _ := newTestService(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "abc.def"})
	protected(svc).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401 got %d", rr.Code)
	}
}

func TestRequireAuthValidAPIKey(t *testing.T) {
	t.Parallel()
	svc, store, uid := newTestService(t)
	ctx := context.Background()
	key, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAPIKey(ctx, auth.CreateAPIKeyParams{
		UserID: uid, Name: "test", KeyHash: hash, Prefix: prefix,
	}); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Api-Key", key)
	protected(svc).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200 got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"method":"apikey"`) {
		t.Fatalf("expected apikey method, got %s", rr.Body.String())
	}
}

func TestRequireAuthExpiredAPIKey(t *testing.T) {
	t.Parallel()
	svc, store, uid := newTestService(t)
	ctx := context.Background()
	key, hash, prefix, _ := auth.GenerateAPIKey()
	past := time.Now().Add(-time.Hour)
	if _, err := store.CreateAPIKey(ctx, auth.CreateAPIKeyParams{
		UserID: uid, Name: "expired", KeyHash: hash, Prefix: prefix, ExpiresAt: &past,
	}); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Api-Key", key)
	protected(svc).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401 got %d", rr.Code)
	}
}

func TestRequireAuthBadAPIKey(t *testing.T) {
	t.Parallel()
	svc, _, _ := newTestService(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Api-Key", "loom_thiskeydoesnotexistinthedb!!")
	protected(svc).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401 got %d", rr.Code)
	}
}

func TestRequireAuthProxyTrusted(t *testing.T) {
	t.Parallel()
	svc, _, _ := newTestService(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Remote-User", "bob")
	req.Header.Set("Remote-Email", "bob@example.com")
	req.Header.Set("Remote-Groups", "admins,developers")
	protected(svc).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200 got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"method":"proxy"`) {
		t.Fatalf("expected proxy method, got %s", rr.Body.String())
	}
}

func TestRequireAuthProxyUntrusted(t *testing.T) {
	t.Parallel()
	svc, _, _ := newTestService(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	req.Header.Set("Remote-User", "evil")
	protected(svc).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401 got %d", rr.Code)
	}
}

func TestRequireRoleAdmin(t *testing.T) {
	t.Parallel()
	svc, _, uid := newTestService(t)
	ok := svc.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	now := time.Now()
	tok, _ := auth.SignSession(svc.SessionSecret(), auth.SessionPayload{
		UID: uid, IAT: now.Unix(), EXP: now.Add(time.Hour).Unix(),
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: tok})
	ok.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: want 204 got %d", rr.Code)
	}
}
