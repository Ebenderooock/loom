package sabnzbd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

// fakeServer is a tiny httptest.Server harness shared by the SABnzbd
// tests. Each test installs a per-mode handler so assertions stay
// next to the table they exercise.
type fakeServer struct {
	t        *testing.T
	srv      *httptest.Server
	expected string
	handlers map[string]http.HandlerFunc
	calls    map[string]int
	lastForm map[string]url.Values
}

func newFakeServer(t *testing.T, apikey string) *fakeServer {
	t.Helper()
	f := &fakeServer{
		t:        t,
		expected: apikey,
		handlers: make(map[string]http.HandlerFunc),
		calls:    make(map[string]int),
		lastForm: make(map[string]url.Values),
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.dispatch))
	return f
}

func (f *fakeServer) dispatch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		_ = r.ParseForm()
	}
	mode := r.URL.Query().Get("mode")
	got := r.URL.Query().Get("apikey")
	if got != f.expected {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":false,"error":"API Key Incorrect"}`)
		return
	}
	if r.URL.Query().Get("output") != "json" {
		f.t.Errorf("expected output=json, got %q", r.URL.Query().Get("output"))
	}
	f.calls[mode]++
	form := url.Values{}
	for k, v := range r.URL.Query() {
		form[k] = v
	}
	for k, v := range r.PostForm {
		form[k] = v
	}
	if r.MultipartForm != nil {
		for k, v := range r.MultipartForm.Value {
			form[k] = v
		}
	}
	f.lastForm[mode] = form

	h, ok := f.handlers[mode]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":true}`)
		return
	}
	h(w, r)
}

func (f *fakeServer) on(mode string, body string) {
	f.handlers[mode] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

func (f *fakeServer) onFunc(mode string, h http.HandlerFunc) {
	f.handlers[mode] = h
}

func (f *fakeServer) close() { f.srv.Close() }

// newTestClient pins the Client at the fake server and installs a
// matching httpClientFactory so kind.go's factory could also be
// exercised end-to-end if a test needs it.
func newTestClient(t *testing.T, f *fakeServer) *Client {
	t.Helper()
	u, err := url.Parse(f.srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	cfg := Config{
		Host:     host,
		Port:     port,
		TLS:      false,
		BasePath: "/",
		APIKey:   f.expected,
	}
	return NewClient("sab-1", "Test SAB", cfg, f.srv.Client())
}

// --- client_test.go cases -------------------------------------------------

func TestClient_VersionProbe(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "secret")
	defer f.close()
	f.on("version", `{"version":"3.7.2"}`)

	c := newTestClient(t, f)
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if f.calls["version"] != 1 {
		t.Fatalf("expected one version call, got %d", f.calls["version"])
	}
}

func TestClient_VersionProbeEmpty(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "secret")
	defer f.close()
	f.on("version", `{"version":""}`)

	c := newTestClient(t, f)
	err := c.Test(context.Background())
	if !errors.Is(err, ErrServer) {
		t.Fatalf("expected ErrServer, got %v", err)
	}
}

func TestClient_APIKeyRejection(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "secret")
	defer f.close()

	cfg := Config{APIKey: "wrong"}
	u, _ := url.Parse(f.srv.URL)
	cfg.Host = u.Hostname()
	cfg.Port, _ = strconv.Atoi(u.Port())
	cfg.BasePath = "/"
	c := NewClient("bad", "Bad", cfg, f.srv.Client())

	err := c.Test(context.Background())
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("expected ErrAuth, got %v", err)
	}
}

func TestClient_HTTP500MapsToUpstream(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "secret")
	defer f.close()
	f.onFunc("version", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	c := newTestClient(t, f)
	err := c.Test(context.Background())
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("expected ErrUpstream, got %v", err)
	}
}

func TestClient_ServerErrorEnvelope(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "secret")
	defer f.close()
	f.on("version", `{"status":false,"error":"unknown mode"}`)

	c := newTestClient(t, f)
	err := c.Test(context.Background())
	if !errors.Is(err, ErrServer) {
		t.Fatalf("expected ErrServer, got %v", err)
	}
	if !strings.Contains(err.Error(), "unknown mode") {
		t.Fatalf("error should include upstream message, got %q", err.Error())
	}
}

func TestClient_BasePathHonored(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	hit := false
	mux.HandleFunc("/sab/api", func(w http.ResponseWriter, r *http.Request) {
		hit = true
		_, _ = io.WriteString(w, `{"version":"3.7.2"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := Config{Host: u.Hostname(), Port: port, BasePath: "/sab", APIKey: "k"}
	c := NewClient("id", "n", cfg, srv.Client())
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !hit {
		t.Fatal("expected server to receive request at /sab/api")
	}
}

// confirm the kind registers itself.
func TestKind_Registered(t *testing.T) {
	t.Parallel()
	if _, err := downloads.LookupKind(downloads.KindSABnzbd); err != nil {
		t.Fatalf("KindSABnzbd not registered: %v", err)
	}
}

// confirm parseConfig round-trips.
func TestParseConfig(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(Config{Host: "h", Port: 8080, APIKey: "k", BasePath: "/sab"})
	cfg, err := parseConfig(raw)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Host != "h" || cfg.APIKey != "k" || cfg.Port != 8080 || cfg.BasePath != "/sab" {
		t.Fatalf("round-trip mismatch: %+v", cfg)
	}

	cfg, err = parseConfig(nil)
	if err != nil || cfg.Host != "" {
		t.Fatalf("nil should yield zero Config: %+v err=%v", cfg, err)
	}
}
