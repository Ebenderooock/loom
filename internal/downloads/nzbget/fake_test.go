package nzbget

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/loomctl/loom/internal/downloads"
)

// fakeServer is a tiny httptest harness that emulates the JSON-RPC
// surface NZBGet exposes. Each test installs a per-method handler
// so assertions stay next to the table they exercise.
type fakeServer struct {
	t        *testing.T
	srv      *httptest.Server
	user     string
	pass     string
	mu       sync.Mutex
	handlers map[string]func(t *testing.T, params []any) (result any, rpcErr *rpcError, httpStatus int)
	calls    map[string]int
	lastReq  map[string][]any
}

func newFakeServer(t *testing.T, user, pass string) *fakeServer {
	t.Helper()
	f := &fakeServer{
		t:        t,
		user:     user,
		pass:     pass,
		handlers: make(map[string]func(t *testing.T, params []any) (any, *rpcError, int)),
		calls:    make(map[string]int),
		lastReq:  make(map[string][]any),
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.dispatch))
	return f
}

func (f *fakeServer) close() { f.srv.Close() }

func (f *fakeServer) on(method string, result any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[method] = func(_ *testing.T, _ []any) (any, *rpcError, int) {
		return result, nil, 200
	}
}

func (f *fakeServer) onErr(method string, err *rpcError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[method] = func(_ *testing.T, _ []any) (any, *rpcError, int) {
		return nil, err, 200
	}
}

func (f *fakeServer) onFunc(method string, fn func(t *testing.T, params []any) (any, *rpcError, int)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[method] = fn
}

func (f *fakeServer) dispatch(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/jsonrpc" {
		http.NotFound(w, r)
		return
	}
	user, pass, ok := r.BasicAuth()
	if !ok || user != f.user || pass != f.pass {
		w.Header().Set("WWW-Authenticate", `Basic realm="NZBGet"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	f.calls[req.Method]++
	f.lastReq[req.Method] = req.Params
	h, ok := f.handlers[req.Method]
	f.mu.Unlock()

	if !ok {
		// Default: report an empty success so unhandled methods do
		// not blow tests up; tests that care assert on calls/lastReq.
		writeRPC(w, req.ID, nil, nil)
		return
	}
	res, rpcErr, status := h(f.t, req.Params)
	if status >= 400 {
		http.Error(w, "boom", status)
		return
	}
	writeRPC(w, req.ID, res, rpcErr)
}

func writeRPC(w http.ResponseWriter, id int, result any, rpcErr *rpcError) {
	w.Header().Set("Content-Type", "application/json")
	env := struct {
		JSONRPC string    `json:"jsonrpc"`
		ID      int       `json:"id"`
		Result  any       `json:"result"`
		Error   *rpcError `json:"error,omitempty"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   rpcErr,
	}
	_ = json.NewEncoder(w).Encode(env)
}

func newTestClient(t *testing.T, f *fakeServer) *Client {
	t.Helper()
	u, err := url.Parse(f.srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	port, _ := strconv.Atoi(u.Port())
	cfg := Config{
		Host:     u.Hostname(),
		Port:     port,
		TLS:      false,
		BasePath: "/",
		Username: f.user,
		Password: f.pass,
	}
	return NewClient("nzb-1", "Test NZBGet", cfg, f.srv.Client())
}

// helper to fetch the lastReq atomically.
func (f *fakeServer) params(method string) []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]any, len(f.lastReq[method]))
	copy(cp, f.lastReq[method])
	return cp
}

func (f *fakeServer) callCount(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

// confirm the kind registers itself.
func TestKind_Registered(t *testing.T) {
	t.Parallel()
	if _, err := downloads.LookupKind(downloads.KindNZBGet); err != nil {
		t.Fatalf("KindNZBGet not registered: %v", err)
	}
}

// confirm parseConfig round-trips.
func TestParseConfig(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(Config{Host: "h", Port: 6789, Username: "u", Password: "p", BasePath: "/nzb"})
	cfg, err := parseConfig(raw)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Host != "h" || cfg.Username != "u" || cfg.Port != 6789 || cfg.BasePath != "/nzb" {
		t.Fatalf("round-trip mismatch: %+v", cfg)
	}
	cfg, err = parseConfig(nil)
	if err != nil || cfg.Host != "" {
		t.Fatalf("nil should yield zero Config: %+v err=%v", cfg, err)
	}
}
