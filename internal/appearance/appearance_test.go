package appearance

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	return &Store{db: db}
}

func TestStoreDefaultsAndPersistence(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	got, err := store.Get(ctx)
	if err != nil || got != Defaults() {
		t.Fatalf("Get() = %+v, %v; want %+v", got, err, Defaults())
	}
	want := Settings{Theme: "amoled", Accent: "rose", BackgroundURL: "https://example.com/background.jpg?q=1#image", BackgroundDim: 0}
	if err := store.Set(ctx, want); err != nil {
		t.Fatal(err)
	}
	// A new store sees the persisted value without a cache warmup.
	reloaded := &Store{db: store.db}
	got, err = reloaded.Get(ctx)
	if err != nil || got != want {
		t.Fatalf("reloaded Get() = %+v, %v; want %+v", got, err, want)
	}
	if err := reloaded.Set(ctx, Defaults()); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(ctx)
	if err != nil || got != Defaults() {
		t.Fatalf("reset Get() = %+v, %v; want defaults", got, err)
	}
}

func TestSettingsValidation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Settings)
		valid  bool
	}{
		{"defaults", func(s *Settings) {}, true},
		{"light", func(s *Settings) { s.Theme = "light" }, true},
		{"dark", func(s *Settings) { s.Theme = "dark" }, true},
		{"amoled", func(s *Settings) { s.Theme = "amoled" }, true},
		{"teal", func(s *Settings) { s.Accent = "teal" }, true},
		{"blue", func(s *Settings) { s.Accent = "blue" }, true},
		{"rose", func(s *Settings) { s.Accent = "rose" }, true},
		{"amber", func(s *Settings) { s.Accent = "amber" }, true},
		{"zero dim", func(s *Settings) { s.BackgroundDim = 0 }, true},
		{"max dim", func(s *Settings) { s.BackgroundDim = 100 }, true},
		{"http", func(s *Settings) { s.BackgroundURL = "http://example.com/image.jpg" }, true},
		{"https", func(s *Settings) { s.BackgroundURL = "https://example.com/image.jpg" }, true},
		{"url length boundary", func(s *Settings) {
			const prefix = "https://example.com/"
			s.BackgroundURL = prefix + strings.Repeat("a", 2048-len(prefix))
		}, true},
		{"unknown theme", func(s *Settings) { s.Theme = "neon" }, false},
		{"empty theme", func(s *Settings) { s.Theme = "" }, false},
		{"unknown accent", func(s *Settings) { s.Accent = "purple" }, false},
		{"empty accent", func(s *Settings) { s.Accent = "" }, false},
		{"negative dim", func(s *Settings) { s.BackgroundDim = -1 }, false},
		{"excessive dim", func(s *Settings) { s.BackgroundDim = 101 }, false},
		{"relative url", func(s *Settings) { s.BackgroundURL = "/image.jpg" }, false},
		{"protocol relative", func(s *Settings) { s.BackgroundURL = "//example.com/image.jpg" }, false},
		{"data url", func(s *Settings) { s.BackgroundURL = "data:image/png;base64,abc" }, false},
		{"javascript url", func(s *Settings) { s.BackgroundURL = "javascript:alert(1)" }, false},
		{"ftp url", func(s *Settings) { s.BackgroundURL = "ftp://example.com/image.jpg" }, false},
		{"missing host", func(s *Settings) { s.BackgroundURL = "https:///image.jpg" }, false},
		{"empty hostname", func(s *Settings) { s.BackgroundURL = "https://:443/image.jpg" }, false},
		{"opaque url", func(s *Settings) { s.BackgroundURL = "https:image.jpg" }, false},
		{"credentials", func(s *Settings) { s.BackgroundURL = "https://user:password@example.com/image.jpg" }, false},
		{"username only", func(s *Settings) { s.BackgroundURL = "https://user@example.com/image.jpg" }, false},
		{"empty credentials", func(s *Settings) { s.BackgroundURL = "https://@example.com/image.jpg" }, false},
		{"malformed url", func(s *Settings) { s.BackgroundURL = "https://example.com/%zz" }, false},
		{"url whitespace", func(s *Settings) { s.BackgroundURL = " https://example.com/image.jpg" }, false},
		{"url too long", func(s *Settings) { s.BackgroundURL = "https://example.com/" + strings.Repeat("a", 2030) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := Defaults()
			tt.change(&settings)
			if err := settings.Validate(); (err == nil) != tt.valid {
				t.Fatalf("Validate() = %v; want valid=%v", err, tt.valid)
			}
		})
	}
}

func TestStoreRejectsInvalidSettings(t *testing.T) {
	store := testStore(t)
	settings := Defaults()
	settings.BackgroundDim = 101
	if err := store.Set(context.Background(), settings); err == nil {
		t.Fatal("expected invalid settings to be rejected")
	}
	got, err := store.Get(context.Background())
	if err != nil || got != Defaults() {
		t.Fatalf("invalid Set changed settings: %+v, %v", got, err)
	}
}

func TestRouterRoundTrip(t *testing.T) {
	store := testStore(t)
	router := Router(store, nil, nil)
	want := Settings{Theme: "dark", Accent: "blue", BackgroundURL: "https://example.com/bg.png", BackgroundDim: 100}
	body, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", w.Code, w.Body.String())
	}
	for _, method := range []string{http.MethodPut, http.MethodGet} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, "/", bytes.NewReader(body)))
		var got Settings
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK || got != want {
			t.Fatalf("%s = %d, %+v; want %+v", method, w.Code, got, want)
		}
		if w.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content type: %s", w.Header().Get("Content-Type"))
		}
	}
}

func TestRouterRejectsInvalidBodiesWithoutWriting(t *testing.T) {
	store := testStore(t)
	router := Router(store, nil, nil)
	valid := `{"theme":"system","accent":"default","backgroundUrl":"","backgroundDim":70}`
	tests := []struct {
		name string
		body string
		code int
	}{
		{"empty", "", 400},
		{"null", "null", 400},
		{"array", "[]", 400},
		{"partial", `{"theme":"dark"}`, 400},
		{"null field", strings.Replace(valid, `"backgroundDim":70`, `"backgroundDim":null`, 1), 400},
		{"unknown field", strings.Replace(valid, `"theme"`, `"unknown":true,"theme"`, 1), 400},
		{"fractional dim", strings.Replace(valid, `:70`, `:1.5`, 1), 400},
		{"string dim", strings.Replace(valid, `:70`, `:"70"`, 1), 400},
		{"invalid theme", strings.Replace(valid, `"system"`, `"neon"`, 1), 400},
		{"invalid accent", strings.Replace(valid, `"default"`, `"neon"`, 1), 400},
		{"dim out of range", strings.Replace(valid, `:70`, `:101`, 1), 400},
		{"unsafe url", strings.Replace(valid, `"backgroundUrl":""`, `"backgroundUrl":"javascript:alert(1)"`, 1), 400},
		{"oversized url", strings.Replace(valid, `"backgroundUrl":""`, `"backgroundUrl":"https://example.com/`+strings.Repeat("a", 2048)+`"`, 1), 400},
		{"trailing object", valid + `{}`, 400},
		{"trailing garbage", valid + `x`, 400},
		{"oversized body", strings.Repeat(" ", maxBodyBytes+1) + valid, 413},
		{"oversized trailing whitespace", valid + strings.Repeat(" ", maxBodyBytes), 413},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(tt.body)))
			if w.Code != tt.code {
				t.Fatalf("status = %d, want %d; body = %s", w.Code, tt.code, w.Body.String())
			}
			got, err := store.Get(context.Background())
			if err != nil || got != Defaults() {
				t.Fatalf("invalid PUT changed settings: %+v, %v", got, err)
			}
		})
	}
}

func TestStorageFailuresAreLoggedAndReported(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			store := testStore(t)
			if err := store.db.Close(); err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			router := Router(store, slog.New(slog.NewTextHandler(&logs, nil)), nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(method, "/", strings.NewReader(
				`{"theme":"dark","accent":"teal","backgroundUrl":"","backgroundDim":0}`)))
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			if !strings.Contains(logs.String(), "level=ERROR") || !strings.Contains(logs.String(), "database is closed") {
				t.Fatalf("missing storage error log: %s", logs.String())
			}
			if strings.Contains(w.Body.String(), "database is closed") {
				t.Fatal("response leaked internal storage error")
			}
		})
	}
}

func TestCorruptStoredSettingsFail(t *testing.T) {
	for _, raw := range []string{"not json", `{"theme":"neon"}`} {
		t.Run(raw, func(t *testing.T) {
			store := testStore(t)
			if _, err := store.db.Exec(`INSERT INTO schema_meta (key, value) VALUES (?, ?)`, settingsKey, raw); err != nil {
				t.Fatal(err)
			}
			router := Router(store, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
		})
	}
}
