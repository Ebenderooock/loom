package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loomctl/loom/internal/kernel/config"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	t.Setenv("LOOM_CONFIG_DIR", t.TempDir())
	t.Setenv("LOOM_DATA_DIR", t.TempDir())
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	s, err := New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
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

func TestMetricsExposed(t *testing.T) {
	s := newTestServer(t)
	w := do(t, s.newMux(), http.MethodGet, "/metrics")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}
