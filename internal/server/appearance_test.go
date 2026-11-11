package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
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

	// Background upload: authenticated reads, admin-only writes, permission
	// checked before the body is read.
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	uploadReq := func(role string, body io.Reader) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		part, _ := mw.CreateFormFile("image", "bg.png")
		_, _ = part.Write(img.Bytes())
		_ = mw.Close()
		if body == nil {
			body = &buf
		}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/appearance/background", body)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		if role != "" {
			r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: cookies[role]})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	getBackground := func(role, target string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		if role != "" {
			r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: cookies[role]})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := uploadReq("", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous upload = %d", w.Code)
	}
	if w := getBackground("", "/api/v1/appearance/background"); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous background GET = %d", w.Code)
	}
	for _, role := range []string{"user", "viewer"} {
		body := &readTracker{}
		if w := uploadReq(role, body); w.Code != http.StatusForbidden || body.read {
			t.Fatalf("%s upload = %d, body read = %v", role, w.Code, body.read)
		}
	}
	w := uploadReq("admin", nil)
	var uploaded struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &uploaded); err != nil || w.Code != http.StatusOK {
		t.Fatalf("admin upload = %d, %s", w.Code, w.Body.String())
	}
	if _, ok := appearance.InternalBackground(uploaded.URL); !ok || !strings.Contains(uploaded.URL, "?v=") {
		t.Fatalf("unexpected upload URL %q", uploaded.URL)
	}
	for _, role := range []string{"admin", "user", "viewer"} {
		w := getBackground(role, uploaded.URL)
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), img.Bytes()) || w.Header().Get("Content-Type") != "image/png" {
			t.Fatalf("%s background GET = %d %q", role, w.Code, w.Header().Get("Content-Type"))
		}
	}
}

type readTracker struct{ read bool }

func (r *readTracker) Read([]byte) (int, error) {
	r.read = true
	return 0, io.EOF
}
