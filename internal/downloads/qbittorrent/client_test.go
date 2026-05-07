package qbittorrent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

// newTestClient assembles a Client whose HTTP layer points at srv.
// All tests in the package share this constructor.
func newTestClient(t *testing.T, srv *httptest.Server, def downloads.Definition) *Client {
	t.Helper()
	if def.ID == "" {
		def.ID = "qb-test"
	}
	if def.Name == "" {
		def.Name = "QB Test"
	}
	if def.Username == "" {
		def.Username = "admin"
	}
	if def.Password == "" {
		def.Password = "adminadmin"
	}
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	host := u.Hostname()
	port := 0
	fmt.Sscanf(u.Port(), "%d", &port)
	def.Host = host
	def.Port = port
	def.TLS = u.Scheme == "https"

	cfg, err := parseConfig(def)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	c, err := NewWithHTTPClient(def, cfg, srv.Client())
	if err != nil {
		t.Fatalf("NewWithHTTPClient: %v", err)
	}
	return c
}

// fakeServer composes an http.ServeMux that mints a session cookie
// on /auth/login and refuses unauthenticated requests on the other
// endpoints. Each test wires its own handler for the endpoint under
// test on top of this base.
type fakeServer struct {
	mux           *http.ServeMux
	srv           *httptest.Server
	loginCalls    atomic.Int64
	loginPassword string
	authzPath     string
}

func newFakeServer(password string) *fakeServer {
	f := &fakeServer{
		mux:           http.NewServeMux(),
		loginPassword: password,
	}
	f.mux.HandleFunc("/api/v2/auth/login", f.handleLogin)
	f.srv = httptest.NewServer(f.mux)
	return f
}

func (f *fakeServer) Close() { f.srv.Close() }

func (f *fakeServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	f.loginCalls.Add(1)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if r.PostFormValue("password") != f.loginPassword {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "Fails.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "SID",
		Value:    "test-session-cookie",
		Path:     "/",
		HttpOnly: true,
	})
	fmt.Fprint(w, "Ok.")
}

// requireSID returns a wrapper that 403s any request whose cookie
// jar does not carry an SID set by the login handler.
func (f *fakeServer) requireSID(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("SID")
		if err != nil || c.Value != "test-session-cookie" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "Forbidden")
			return
		}
		h(w, r)
	}
}

func TestLoginSuccess(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	f.mux.HandleFunc("/api/v2/app/version", f.requireSID(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "v4.6.5")
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if got := f.loginCalls.Load(); got != 1 {
		t.Fatalf("login calls = %d, want 1", got)
	}
}

func TestLoginRejected(t *testing.T) {
	t.Parallel()
	f := newFakeServer("the-real-password")
	defer f.Close()

	c := newTestClient(t, f.srv, downloads.Definition{Password: "wrong"})
	err := c.Test(context.Background())
	if err == nil {
		t.Fatal("Test: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("error %q does not mention auth failure", err)
	}
}

func TestSessionCookieIsReused(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	var versionHits atomic.Int64
	f.mux.HandleFunc("/api/v2/app/version", f.requireSID(func(w http.ResponseWriter, _ *http.Request) {
		versionHits.Add(1)
		fmt.Fprint(w, "v5.0.0")
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	for i := 0; i < 3; i++ {
		if err := c.Test(context.Background()); err != nil {
			t.Fatalf("Test #%d: %v", i, err)
		}
	}
	// Test() always re-logs in (it's a credential check), so we
	// expect 3 logins. The point of this test is that downstream
	// /app/version calls succeed thanks to the cookie set by login.
	if got := f.loginCalls.Load(); got != 3 {
		t.Fatalf("login calls = %d, want 3", got)
	}
	if got := versionHits.Load(); got != 3 {
		t.Fatalf("version calls = %d, want 3", got)
	}
}

func TestReLoginOn403(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()

	// /torrents/info returns 403 for the first call (simulating
	// an expired SID), then 200.
	var infoHits atomic.Int64
	f.mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, r *http.Request) {
		hit := infoHits.Add(1)
		if hit == 1 {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "Forbidden")
			return
		}
		// On retry the SID cookie must be present.
		c, err := r.Cookie("SID")
		if err != nil || c.Value != "test-session-cookie" {
			t.Errorf("retry missing cookie: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
	})

	c := newTestClient(t, f.srv, downloads.Definition{})
	// Pre-seed by calling login once so the jar has a cookie.
	if err := c.login(context.Background()); err != nil {
		t.Fatalf("seed login: %v", err)
	}
	preLogin := f.loginCalls.Load()

	if _, err := c.Status(context.Background()); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got := infoHits.Load(); got != 2 {
		t.Fatalf("info hits = %d, want 2 (initial 403 + retry)", got)
	}
	if got := f.loginCalls.Load(); got != preLogin+1 {
		t.Fatalf("login calls = %d, want %d (one re-login)", got, preLogin+1)
	}
}

func TestParseConfigPrefersConfigOverDefinition(t *testing.T) {
	t.Parallel()
	def := downloads.Definition{
		Host:     "definition-host",
		Port:     8080,
		Username: "def-user",
		Password: "def-pass",
		Config: mustJSON(t, Config{
			Host:     "config-host",
			Port:     9000,
			TLS:      true,
			BasePath: "/qbt",
			Username: "cfg-user",
			Password: "cfg-pass",
		}),
	}
	r, err := parseConfig(def)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if got, want := r.baseURL.String(), "https://config-host:9000/qbt/"; got != want {
		t.Fatalf("baseURL = %q, want %q", got, want)
	}
	if r.username != "cfg-user" || r.password != "cfg-pass" {
		t.Fatalf("credentials = %q/%q, want cfg-user/cfg-pass", r.username, r.password)
	}
}

func TestParseConfigRejectsMissingHost(t *testing.T) {
	t.Parallel()
	_, err := parseConfig(downloads.Definition{})
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
