package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
	"github.com/ebenderooock/loom/internal/auth"
)

func TestUpdateRoutesAdminOnly(t *testing.T) {
	s := newTestServer(t)
	store, err := auth.StoreFromDB(s.db)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("update-test-session-secret-32-bytes")
	s.authSvc, err = auth.NewService(auth.ServiceOptions{
		Store: store, Logger: s.logger, AppConfig: &appconfig.Config{},
		AppConfigPath: filepath.Join(t.TempDir(), "config.json"), SessionSecret: secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	router := s.newMux()
	for _, role := range []string{"", "admin", "user", "viewer"} {
		var cookie string
		if role != "" {
			user, err := store.CreateUser(context.Background(), auth.CreateUserParams{
				Username: role, PasswordHash: "unused-session-test-hash", Role: role,
			})
			if err != nil {
				t.Fatal(err)
			}
			cookie, err = auth.SignSession(secret, auth.SessionPayload{
				UID: user.ID, IAT: time.Now().Unix(), EXP: time.Now().Add(time.Hour).Unix(),
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, route := range []struct{ method, path string }{
			{"GET", "/api/v1/updates"}, {"PUT", "/api/v1/updates"}, {"POST", "/api/v1/updates/check"},
		} {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"channel":"stable"}`))
			if cookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: cookie})
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			want := http.StatusForbidden
			if role == "" {
				want = http.StatusUnauthorized
			}
			if role == "admin" {
				want = http.StatusOK
			}
			if w.Code != want {
				t.Fatalf("%s %s as %s = %d, want %d: %s", route.method, route.path, role, w.Code, want, w.Body.String())
			}
		}
	}
}
