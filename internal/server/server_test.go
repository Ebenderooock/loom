package server

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/kernel/telemetry"
	"github.com/loomctl/loom/internal/storage"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	t.Setenv("LOOM_CONFIG_DIR", t.TempDir())
	t.Setenv("LOOM_DATA_DIR", t.TempDir())
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Storage = config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: t.TempDir() + "/loom.db"},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tel, err := telemetry.New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(context.Background(), cfg.Storage, logger)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(cfg, logger, tel, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
		_ = db.Close()
		_ = tel.Shutdown(context.Background())
	})
	return s
}

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t)
	w := do(t, s.newMux(), http.MethodGet, "/healthz")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"ok"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestReadyzBeforeStartReportsStarting(t *testing.T) {
	s := newTestServer(t)
	w := do(t, s.newMux(), http.MethodGet, "/readyz")
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestReadyzAfterStartReportsReady(t *testing.T) {
	s := newTestServer(t)
	s.ready.Store(true)
	w := do(t, s.newMux(), http.MethodGet, "/readyz")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}

func TestSystemStatusIncludesVersion(t *testing.T) {
	s := newTestServer(t)
	w := do(t, s.newMux(), http.MethodGet, "/api/v1/system/status")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"version", "commit", "buildDate", "engine"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing %q in status response: %v", k, got)
		}
	}
}

func TestSystemStatusEmitsETagAnd304(t *testing.T) {
	s := newTestServer(t)
	mux := s.newMux()
	w := do(t, mux, http.MethodGet, "/api/v1/system/status")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header")
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	r.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, r)
	if w2.Code != http.StatusNotModified {
		t.Errorf("expected 304 with matching ETag, got %d", w2.Code)
	}
}

func TestMetricsExposesPromText(t *testing.T) {
	s := newTestServer(t)
	w := do(t, s.newMux(), http.MethodGet, "/metrics")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "go_goroutines") {
		t.Errorf("metrics output should include process/go runtime collectors; got:\n%s", w.Body.String())
	}
}

func TestRequestIDPropagation(t *testing.T) {
	s := newTestServer(t)
	w := do(t, s.newMux(), http.MethodGet, "/healthz")
	if got := w.Header().Get("X-Request-Id"); got == "" {
		t.Errorf("expected X-Request-Id in response headers")
	}
}

func TestPanicRecoveryReturns500JSON(t *testing.T) {
	s := newTestServer(t)
	// Mount a panicking route on a fresh router that reuses the server's
	// middleware chain. Easiest: build chi router and apply recoverer.
	r := chi.NewRouter()
	r.Use(s.recoverer)
	r.Get("/boom", func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	})

	w := do(t, r, http.MethodGet, "/boom")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected JSON content-type, got %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not json: %v: %s", err, w.Body.String())
	}
	if body["error"] == "" {
		t.Errorf("expected error field in body: %v", body)
	}
}

func TestGzipCompression(t *testing.T) {
	s := newTestServer(t)
	mux := s.newMux()

	r := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip Content-Encoding, headers=%v", w.Header())
	}
	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	dec, err := io.ReadAll(gr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dec), "version") {
		t.Errorf("decoded body missing 'version': %s", string(dec))
	}
}

func TestCORSPreflightWhenConfigured(t *testing.T) {
	t.Setenv("LOOM_CONFIG_DIR", t.TempDir())
	t.Setenv("LOOM_DATA_DIR", t.TempDir())
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CORS.AllowedOrigins = []string{"https://example.com"}
	cfg.Storage = config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: t.TempDir() + "/loom.db"},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tel, err := telemetry.New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Shutdown(context.Background()) })
	db, err := storage.Open(context.Background(), cfg.Storage, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := New(cfg, logger, tel, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })

	mux := s.newMux()
	r := httptest.NewRequest(http.MethodOptions, "/api/v1/system/status", nil)
	r.Header.Set("Origin", "https://example.com")
	r.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Errorf("missing Access-Control-Allow-Methods")
	}

	// Disallowed origin should not echo header.
	r2 := httptest.NewRequest(http.MethodOptions, "/api/v1/system/status", nil)
	r2.Header.Set("Origin", "https://evil.example.org")
	r2.Header.Set("Access-Control-Request-Method", "GET")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, r2)
	if got := w2.Header().Get("Access-Control-Allow-Origin"); got == "https://evil.example.org" {
		t.Errorf("disallowed origin was echoed back")
	}
}

func TestPprofGated(t *testing.T) {
	t.Setenv("LOOM_CONFIG_DIR", t.TempDir())
	t.Setenv("LOOM_DATA_DIR", t.TempDir())
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Storage = config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: t.TempDir() + "/loom.db"},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tel, err := telemetry.New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Shutdown(context.Background()) })
	db, err := storage.Open(context.Background(), cfg.Storage, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Default: pprof disabled.
	s, err := New(cfg, logger, tel, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	w := do(t, s.newMux(), http.MethodGet, "/debug/pprof/")
	if w.Code != http.StatusNotFound {
		t.Errorf("pprof should be 404 by default; got %d", w.Code)
	}

	// Enabled.
	cfg2, _ := config.Load("")
	cfg2.Debug.Pprof = true
	cfg2.Storage = cfg.Storage
	s2, err := New(cfg2, logger, tel, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s2.Shutdown(context.Background()) })
	w2 := do(t, s2.newMux(), http.MethodGet, "/debug/pprof/")
	if w2.Code != http.StatusOK {
		t.Errorf("pprof should be 200 when enabled; got %d", w2.Code)
	}
}
