package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
	"github.com/ebenderooock/loom/internal/appearance"
	"github.com/ebenderooock/loom/internal/auth"
)

func TestAppearanceAuthenticatedRoutes(t *testing.T) {
	s := newTestServer(t)
	store, err := auth.StoreFromDB(s.db)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("appearance-test-session-secret-32")
	s.authSvc, err = auth.NewService(auth.ServiceOptions{
		Store:         store,
		Logger:        s.logger,
		AppConfig:     &appconfig.Config{},
		AppConfigPath: filepath.Join(t.TempDir(), "config.json"),
		SessionSecret: secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	cookies := map[string]string{}
	for _, role := range []string{"admin", "user", "viewer"} {
		user, err := store.CreateUser(context.Background(), auth.CreateUserParams{
			Username: role, PasswordHash: "unused-session-test-hash", Role: role,
		})
		if err != nil {
			t.Fatal(err)
		}
		value, err := auth.SignSession(secret, auth.SessionPayload{
			UID: user.ID, IAT: time.Now().Unix(), EXP: time.Now().Add(time.Hour).Unix(),
		})
		if err != nil {
			t.Fatal(err)
		}
		cookies[role] = value
	}
	router := s.newMux()
	body := `{"theme":"amoled","accent":"amber","backgroundUrl":"https://example.com/bg.jpg","backgroundDim":42}`
	request := func(method, role string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/api/v1/appearance", strings.NewReader(body))
		if role != "" {
			r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: cookies[role]})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		if w := request(method, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s = %d, body = %s", method, w.Code, w.Body.String())
		}
	}
	for _, role := range []string{"admin", "user", "viewer"} {
		w := request(http.MethodGet, role)
		var got appearance.Settings
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK || got != appearance.Defaults() {
			t.Fatalf("%s GET = %d, %+v; want defaults", role, w.Code, got)
		}
	}
	for _, role := range []string{"user", "viewer"} {
		if w := request(http.MethodPut, role); w.Code != http.StatusForbidden {
			t.Fatalf("%s PUT = %d, body = %s", role, w.Code, w.Body.String())
		}
	}
	if w := request(http.MethodPut, "admin"); w.Code != http.StatusOK {
		t.Fatalf("admin PUT = %d, body = %s", w.Code, w.Body.String())
	}
	for _, role := range []string{"admin", "user", "viewer"} {
		w := request(http.MethodGet, role)
		var got appearance.Settings
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		want := appearance.Settings{Theme: "amoled", Accent: "amber", BackgroundURL: "https://example.com/bg.jpg", BackgroundDim: 42}
		if w.Code != http.StatusOK || got != want {
			t.Fatalf("%s GET after admin PUT = %d, %+v; want %+v", role, w.Code, got, want)
		}
	}
}
