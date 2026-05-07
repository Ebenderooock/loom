package transmission

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

// fakeDaemon is a minimal Transmission RPC stand-in. It enforces the
// session-id handshake (rejecting requests with a stale or missing
// id) and dispatches by method. Tests register handlers via
// f.handle(method, fn).
type fakeDaemon struct {
	t *testing.T

	srv *httptest.Server

	mu        sync.Mutex
	sessionID string
	username  string
	password  string
	skipAuth  bool
	handlers  map[string]func(args json.RawMessage) (any, string)

	conflicts atomic.Int64
	calls     atomic.Int64
}

func newFakeDaemon(t *testing.T) *fakeDaemon {
	t.Helper()
	f := &fakeDaemon{
		t:         t,
		sessionID: "transmission-session-zero",
		handlers:  make(map[string]func(args json.RawMessage) (any, string)),
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeDaemon) Close() { f.srv.Close() }

// handle is the http.HandlerFunc bolted onto httptest.NewServer.
func (f *fakeDaemon) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != defaultRPCURL {
		http.NotFound(w, r)
		return
	}
	if !f.skipAuth && (f.username != "" || f.password != "") {
		u, p, ok := r.BasicAuth()
		if !ok || u != f.username || p != f.password {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	f.mu.Lock()
	currentID := f.sessionID
	f.mu.Unlock()
	if got := r.Header.Get(sessionHeader); got != currentID {
		w.Header().Set(sessionHeader, currentID)
		f.conflicts.Add(1)
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, "409: Conflict.")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var env rpcRequest
	if err := json.Unmarshal(body, &env); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.calls.Add(1)

	handler, ok := f.handlers[env.Method]
	if !ok {
		writeRPC(w, "method not registered: "+env.Method, nil)
		return
	}
	rawArgs, _ := json.Marshal(env.Arguments)
	out, result := handler(rawArgs)
	if result == "" {
		result = "success"
	}
	writeRPC(w, result, out)
}

func writeRPC(w http.ResponseWriter, result string, args any) {
	body := map[string]any{"result": result}
	if args != nil {
		body["arguments"] = args
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// rotateSession invalidates the current session id so the next
// request triggers a 409 handshake.
func (f *fakeDaemon) rotateSession(id string) {
	f.mu.Lock()
	f.sessionID = id
	f.mu.Unlock()
}

func (f *fakeDaemon) handle_(method string, fn func(args json.RawMessage) (any, string)) {
	f.handlers[method] = fn
}

// newTestClient wires a Client whose transport hits f.srv, using the
// configured username/password.
func newTestClient(t *testing.T, f *fakeDaemon, def downloads.Definition) *Client {
	t.Helper()
	if def.ID == "" {
		def.ID = "transmission-test"
	}
	if def.Name == "" {
		def.Name = "Transmission Test"
	}
	u, err := url.Parse(f.srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	def.Host = u.Hostname()
	port := 0
	fmt.Sscanf(u.Port(), "%d", &port)
	def.Port = port
	def.TLS = u.Scheme == "https"

	cfg, err := parseConfig(def)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	c, err := NewWithHTTPClient(def, cfg, f.srv.Client())
	if err != nil {
		t.Fatalf("NewWithHTTPClient: %v", err)
	}
	return c
}

// TestSessionHandshakeOn409 verifies the canonical Transmission CSRF
// handshake: the first request lacks a session id, the daemon
// responds with 409 + X-Transmission-Session-Id, and the client
// replays once with the captured id.
func TestSessionHandshakeOn409(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()
	f.handle_("session-get", func(_ json.RawMessage) (any, string) {
		return sessionInfo{Version: "4.0.5", RPCVer: 17, RPCMin: 14}, "success"
	})

	c := newTestClient(t, f, downloads.Definition{})
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if got := f.conflicts.Load(); got != 1 {
		t.Fatalf("conflicts = %d, want 1 (the initial handshake)", got)
	}
	if got := f.calls.Load(); got != 1 {
		t.Fatalf("successful calls = %d, want 1", got)
	}
	// A subsequent call should reuse the stored id with no extra 409.
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("second Test: %v", err)
	}
	if got := f.conflicts.Load(); got != 1 {
		t.Fatalf("conflicts after 2 calls = %d, want 1", got)
	}
}

// TestSessionRotationReHandshakes verifies the client picks up a new
// id when the daemon rotates it mid-flight.
func TestSessionRotationReHandshakes(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()
	f.handle_("session-get", func(_ json.RawMessage) (any, string) {
		return sessionInfo{Version: "4.0.5"}, "success"
	})

	c := newTestClient(t, f, downloads.Definition{})
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("first Test: %v", err)
	}
	f.rotateSession("rotated-session-id")
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("second Test after rotation: %v", err)
	}
	if got := f.conflicts.Load(); got != 2 {
		t.Fatalf("conflicts = %d, want 2 (initial + post-rotation)", got)
	}
}

// TestBasicAuthIsPassedThrough verifies HTTP Basic credentials reach
// the daemon and that wrong creds surface ErrAuth.
func TestBasicAuthIsPassedThrough(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()
	f.username = "transmission"
	f.password = "swordfish"
	f.handle_("session-get", func(_ json.RawMessage) (any, string) {
		return sessionInfo{Version: "4.0.5"}, "success"
	})

	good := newTestClient(t, f, downloads.Definition{
		Username: "transmission",
		Password: "swordfish",
	})
	if err := good.Test(context.Background()); err != nil {
		t.Fatalf("Test with good creds: %v", err)
	}

	bad := newTestClient(t, f, downloads.Definition{
		Username: "transmission",
		Password: "wrong",
	})
	err := bad.Test(context.Background())
	if err == nil {
		t.Fatal("Test with wrong creds: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("error %q does not mention auth failure", err)
	}
}

func TestParseConfigPrefersConfigOverDefinition(t *testing.T) {
	t.Parallel()
	def := downloads.Definition{
		Host:     "definition-host",
		Port:     9091,
		Username: "def",
		Password: "def-pass",
		Config: mustJSON(t, Config{
			Host:     "config-host",
			Port:     9092,
			TLS:      true,
			RPCURL:   "/transmission/rpc",
			Username: "cfg",
			Password: "cfg-pass",
		}),
	}
	r, err := parseConfig(def)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if got, want := r.endpoint.String(), "https://config-host:9092/transmission/rpc"; got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
	if r.username != "cfg" || r.password != "cfg-pass" {
		t.Fatalf("credentials = %q/%q, want cfg/cfg-pass", r.username, r.password)
	}
}

func TestParseConfigDefaultsRPCURL(t *testing.T) {
	t.Parallel()
	r, err := parseConfig(downloads.Definition{Host: "h", Port: 9091})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if got, want := r.endpoint.Path, defaultRPCURL; got != want {
		t.Fatalf("default rpc url = %q, want %q", got, want)
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
