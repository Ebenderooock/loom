package deluge

import (
	"context"
	"encoding/json"
	"errors"
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

// newTestClient assembles a Client whose HTTP layer points at srv.
// All tests in the package share this constructor. The default
// password is "deluge" so loginPassword on fakeServer should match.
func newTestClient(t *testing.T, srv *httptest.Server, def downloads.Definition) *Client {
	t.Helper()
	if def.ID == "" {
		def.ID = "deluge-test"
	}
	if def.Name == "" {
		def.Name = "Deluge Test"
	}
	if def.Password == "" {
		def.Password = "deluge"
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

// rpcCall is the decoded body of a single inbound /json request.
type rpcCall struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// fakeServer is the test double for a Deluge Web UI. It dispatches
// /json requests to per-method handlers registered by tests, and
// transparently mints the _session_id cookie on a successful
// auth.login. Methods that require a session are wrapped with
// requireSession.
type fakeServer struct {
	srv      *httptest.Server
	password string

	mu       sync.Mutex
	handlers map[string]rpcHandler

	loginCalls atomic.Int64
}

// rpcHandler returns either a result (encoded into json) or a
// non-nil RPCError. Tests register one per method.
type rpcHandler func(t *testing.T, r *http.Request, params json.RawMessage) (any, *RPCError)

func newFakeServer(t *testing.T, password string) *fakeServer {
	t.Helper()
	f := &fakeServer{
		password: password,
		handlers: map[string]rpcHandler{},
	}
	f.handlers["auth.login"] = f.handleLogin
	f.handlers["auth.check_session"] = f.handleCheckSession
	mux := http.NewServeMux()
	mux.HandleFunc("/json", f.dispatch(t))
	f.srv = httptest.NewServer(mux)
	return f
}

func (f *fakeServer) Close() { f.srv.Close() }

// handle registers a per-method handler, replacing any existing one.
func (f *fakeServer) handle(method string, h rpcHandler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[method] = h
}

// requireSession wraps a handler so it 200s only when the inbound
// request carries the session cookie minted by handleLogin. Methods
// that should be rejected without a session use this wrapper.
func (f *fakeServer) requireSession(h rpcHandler) rpcHandler {
	return func(t *testing.T, r *http.Request, params json.RawMessage) (any, *RPCError) {
		ck, err := r.Cookie(sessionCookieName)
		if err != nil || ck.Value != "test-session" {
			return nil, &RPCError{Code: 1, Message: "Not authenticated"}
		}
		return h(t, r, params)
	}
}

func (f *fakeServer) dispatch(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var call rpcCall
		if err := json.Unmarshal(body, &call); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		f.mu.Lock()
		h, ok := f.handlers[call.Method]
		f.mu.Unlock()
		if !ok {
			writeRPC(w, call.ID, nil, &RPCError{
				Code:    2,
				Message: "Unknown method: " + call.Method,
			})
			return
		}
		result, rpcErr := h(t, r, call.Params)
		writeRPC(w, call.ID, result, rpcErr)
	}
}

func writeRPC(w http.ResponseWriter, id int64, result any, rpcErr *RPCError) {
	env := map[string]any{
		"id":     id,
		"result": result,
		"error":  rpcErr,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(env)
}

func (f *fakeServer) handleLogin(t *testing.T, r *http.Request, params json.RawMessage) (any, *RPCError) {
	t.Helper()
	f.loginCalls.Add(1)
	var args []string
	if err := json.Unmarshal(params, &args); err != nil || len(args) != 1 {
		return false, &RPCError{Code: 3, Message: "Bad params"}
	}
	if args[0] != f.password {
		return false, &RPCError{Code: 4, Message: "Bad Login"}
	}
	// We cannot set on the writer inside the handler signature;
	// dispatch wraps writes after handlers. Workaround: stash
	// cookie issuance via a tiny hook on the dispatcher.
	loginCookieIssue.Store(true)
	return true, nil
}

func (f *fakeServer) handleCheckSession(_ *testing.T, r *http.Request, _ json.RawMessage) (any, *RPCError) {
	ck, err := r.Cookie(sessionCookieName)
	if err != nil || ck.Value == "" {
		return false, nil
	}
	return true, nil
}

// loginCookieIssue is a per-request flag the dispatcher inspects to
// know whether the just-handled login should mint a cookie. Tests
// run sequentially within a single fakeServer's dispatcher, but to
// be safe across t.Parallel test files we use atomic.Bool.
var loginCookieIssue atomic.Bool

// Switch dispatch to consult loginCookieIssue and set the cookie
// before writing. We do this via an init-time replacement of
// dispatch's behaviour using a thin wrapper.
//
// (The tests below use a slightly different — and simpler — approach:
// they register their own auth.login handler when they need cookie
// behaviour. The default handler above is here only to keep the
// fixture useful for the parseConfig-only tests.)

// installCookieMintingLogin is the convenience hook tests use when
// they need a fakeServer that actually issues _session_id on a
// successful login. It replaces the default handleLogin with one
// that also sets the cookie on the response writer.
func (f *fakeServer) installCookieMintingLogin() {
	f.handle("auth.login", func(t *testing.T, r *http.Request, params json.RawMessage) (any, *RPCError) {
		t.Helper()
		f.loginCalls.Add(1)
		var args []string
		if err := json.Unmarshal(params, &args); err != nil || len(args) != 1 {
			return false, &RPCError{Code: 3, Message: "Bad params"}
		}
		if args[0] != f.password {
			return false, &RPCError{Code: 4, Message: "Bad Login"}
		}
		return true, nil
	})
}

// writeRPCWithHook writes an RPC envelope, issuing the session cookie
// when the call was a successful auth.login. The cookie is set per
// response so parallel tests cannot steal each other's sessions.
func writeRPCWithHook(w http.ResponseWriter, method string, id int64, result any, rpcErr *RPCError) {
	if method == "auth.login" && rpcErr == nil && result == true {
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "test-session",
			Path:     "/",
			HttpOnly: true,
		})
	}
	env := map[string]any{
		"id":     id,
		"result": result,
		"error":  rpcErr,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(env)
}

// dispatchWithHook is the variant the test fixture mounts on /json.
// It replaces the in-package dispatch so cookie minting works.
func (f *fakeServer) dispatchWithHook(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var call rpcCall
		if err := json.Unmarshal(body, &call); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		f.mu.Lock()
		h, ok := f.handlers[call.Method]
		f.mu.Unlock()
		if !ok {
			writeRPCWithHook(w, call.Method, call.ID, nil, &RPCError{
				Code:    2,
				Message: "Unknown method: " + call.Method,
			})
			return
		}
		result, rpcErr := h(t, r, call.Params)
		writeRPCWithHook(w, call.Method, call.ID, result, rpcErr)
	}
}

// newFakeServerWithLogin returns a fakeServer that issues real
// _session_id cookies on a successful auth.login. This is what
// every operational test below uses.
func newFakeServerWithLogin(t *testing.T, password string) *fakeServer {
	t.Helper()
	f := &fakeServer{
		password: password,
		handlers: map[string]rpcHandler{},
	}
	f.handlers["auth.check_session"] = f.handleCheckSession
	mux := http.NewServeMux()
	mux.HandleFunc("/json", f.dispatchWithHook(t))
	f.srv = httptest.NewServer(mux)
	f.installCookieMintingLogin()
	return f
}

// ----- The actual tests -----

func TestTestFallsBackToDaemonInfoForDeluge1(t *testing.T) {
	t.Parallel()
	f := newFakeServerWithLogin(t, "secret")
	defer f.Close()
	f.handle("web.connected", f.requireSession(func(_ *testing.T, _ *http.Request, _ json.RawMessage) (any, *RPCError) {
		return true, nil
	}))
	f.handle("daemon.info", f.requireSession(func(_ *testing.T, _ *http.Request, _ json.RawMessage) (any, *RPCError) {
		return "1.3.15", nil
	}))

	c := newTestClient(t, f.srv, downloads.Definition{Password: "secret"})
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
}

func TestLoginSuccessAndCookieReuse(t *testing.T) {
	t.Parallel()
	f := newFakeServerWithLogin(t, "secret")
	defer f.Close()
	var versionHits atomic.Int64
	f.handle("daemon.get_version", f.requireSession(func(_ *testing.T, _ *http.Request, _ json.RawMessage) (any, *RPCError) {
		versionHits.Add(1)
		return "2.2.0", nil
	}))
	f.handle("web.connected", f.requireSession(func(_ *testing.T, _ *http.Request, _ json.RawMessage) (any, *RPCError) {
		return true, nil
	}))

	c := newTestClient(t, f.srv, downloads.Definition{Password: "secret"})
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if got := f.loginCalls.Load(); got != 1 {
		t.Fatalf("login calls = %d, want 1", got)
	}
	if got := versionHits.Load(); got != 1 {
		t.Fatalf("daemon.get_version hits = %d, want 1", got)
	}
	// The cookie jar should hold the session cookie.
	cookies := c.http.Jar.Cookies(c.cfg.baseURL)
	found := false
	for _, ck := range cookies {
		if ck.Name == sessionCookieName && ck.Value == "test-session" {
			found = true
		}
	}
	if !found {
		t.Fatalf("session cookie was not persisted; got %v", cookies)
	}
}

func TestLoginRejected(t *testing.T) {
	t.Parallel()
	f := newFakeServerWithLogin(t, "real-pw")
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

func TestSessionExpiryTriggersReLogin(t *testing.T) {
	t.Parallel()
	f := newFakeServerWithLogin(t, "secret")
	defer f.Close()

	// core.get_torrents_status returns "Not authenticated" on the
	// first call, succeeds on the second once the client has
	// re-logged in.
	var statusHits atomic.Int64
	f.handle("core.get_torrents_status", func(_ *testing.T, r *http.Request, _ json.RawMessage) (any, *RPCError) {
		hit := statusHits.Add(1)
		if hit == 1 {
			return nil, &RPCError{Code: 1, Message: "Not authenticated"}
		}
		ck, err := r.Cookie(sessionCookieName)
		if err != nil || ck.Value != "test-session" {
			return nil, &RPCError{Code: 1, Message: "Not authenticated"}
		}
		return map[string]map[string]any{}, nil
	})

	c := newTestClient(t, f.srv, downloads.Definition{Password: "secret"})
	// Pre-seed cookie via a login so ensureLoggedIn is a no-op
	// and the retry path runs.
	if err := c.login(context.Background()); err != nil {
		t.Fatalf("seed login: %v", err)
	}
	preLogin := f.loginCalls.Load()

	if _, err := c.Status(context.Background()); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got := statusHits.Load(); got != 2 {
		t.Fatalf("status hits = %d, want 2 (initial expiry + retry)", got)
	}
	if got := f.loginCalls.Load(); got != preLogin+1 {
		t.Fatalf("login calls = %d, want %d (one re-login)", got, preLogin+1)
	}
}

func TestRPCErrorIsSurfaced(t *testing.T) {
	t.Parallel()
	f := newFakeServerWithLogin(t, "secret")
	defer f.Close()
	f.handle("core.get_free_space", f.requireSession(func(_ *testing.T, _ *http.Request, _ json.RawMessage) (any, *RPCError) {
		return nil, &RPCError{Code: 5, Message: "Permission denied"}
	}))
	f.handle("core.get_config_value", f.requireSession(func(_ *testing.T, _ *http.Request, _ json.RawMessage) (any, *RPCError) {
		return "/downloads", nil
	}))

	c := newTestClient(t, f.srv, downloads.Definition{Password: "secret"})
	_, err := c.FreeSpace(context.Background())
	if err == nil {
		t.Fatal("FreeSpace: expected error")
	}
	if !strings.Contains(err.Error(), "Permission denied") || !strings.Contains(err.Error(), "code=5") {
		t.Fatalf("error does not surface RPC details: %v", err)
	}
}

func TestParseConfigPrefersConfigOverDefinition(t *testing.T) {
	t.Parallel()
	def := downloads.Definition{
		Host:     "definition-host",
		Port:     8080,
		Password: "def-pass",
		Config: mustJSON(t, Config{
			Host:     "config-host",
			Port:     9000,
			TLS:      true,
			BasePath: "/deluge",
			Password: "cfg-pass",
		}),
	}
	r, err := parseConfig(def)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if got, want := r.baseURL.String(), "https://config-host:9000/deluge/"; got != want {
		t.Fatalf("baseURL = %q, want %q", got, want)
	}
	if r.password != "cfg-pass" {
		t.Fatalf("password = %q, want cfg-pass", r.password)
	}
}

func TestParseConfigRejectsMissingHostAndPassword(t *testing.T) {
	t.Parallel()
	if _, err := parseConfig(downloads.Definition{Password: "x"}); err == nil {
		t.Fatal("expected error for missing host")
	}
	if _, err := parseConfig(downloads.Definition{Host: "h"}); err == nil {
		t.Fatal("expected error for missing password")
	}
}

func TestRPCEndpointHonoursBasePath(t *testing.T) {
	t.Parallel()
	r, err := parseConfig(downloads.Definition{
		Host:     "deluge.example",
		Port:     8112,
		Password: "x",
		Config:   mustJSON(t, Config{BasePath: "/deluge"}),
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	c := &Client{cfg: r}
	got := c.rpcEndpoint()
	want := "http://deluge.example:8112/deluge/json"
	if got != want {
		t.Fatalf("rpcEndpoint = %q, want %q", got, want)
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

func TestConcurrentSessionExpiryLogsInOnce(t *testing.T) {
	t.Parallel()
	f := newFakeServerWithLogin(t, "secret")
	defer f.Close()

	const callers = 8
	var expired atomic.Bool
	var arrived sync.WaitGroup
	arrived.Add(callers)
	var once sync.Once
	release := make(chan struct{})
	f.handle("core.get_torrents_status", func(_ *testing.T, _ *http.Request, _ json.RawMessage) (any, *RPCError) {
		if expired.Load() {
			// Hold every caller until all of them have seen the
			// expired session, so they race to refresh it.
			arrived.Done()
			<-release
			return nil, &RPCError{Code: 1, Message: "Not authenticated"}
		}
		return map[string]map[string]any{}, nil
	})

	c := newTestClient(t, f.srv, downloads.Definition{Password: "secret"})
	if err := c.login(context.Background()); err != nil {
		t.Fatalf("seed login: %v", err)
	}
	preLogin := f.loginCalls.Load()
	expired.Store(true)
	go func() {
		arrived.Wait()
		expired.Store(false)
		once.Do(func() { close(release) })
	}()

	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Status(context.Background())
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
	}
	if got := f.loginCalls.Load() - preLogin; got != 1 {
		t.Fatalf("re-logins = %d, want 1 shared refresh", got)
	}
}

func TestFailedLoginIsNotRetriedDuringCooldown(t *testing.T) {
	t.Parallel()
	f := newFakeServerWithLogin(t, "real-pw")
	defer f.Close()
	f.handle("core.get_torrents_status", f.requireSession(func(_ *testing.T, _ *http.Request, _ json.RawMessage) (any, *RPCError) {
		return map[string]map[string]any{}, nil
	}))

	c := newTestClient(t, f.srv, downloads.Definition{Password: "wrong"})
	for i := range 5 {
		if _, err := c.Status(context.Background()); err == nil {
			t.Fatalf("Status #%d: expected auth error", i)
		} else if !errors.Is(err, ErrAuthFailed) {
			t.Fatalf("Status #%d: error %v does not wrap ErrAuthFailed", i, err)
		}
	}
	if got := f.loginCalls.Load(); got != 1 {
		t.Fatalf("login calls = %d, want 1 during cooldown", got)
	}
	// An explicit connection test always probes the credentials.
	_ = c.Test(context.Background())
	if got := f.loginCalls.Load(); got != 2 {
		t.Fatalf("login calls after Test = %d, want 2", got)
	}
}
