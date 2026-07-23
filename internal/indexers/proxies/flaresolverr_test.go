package proxies_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/indexers/proxies"
)

func TestFlareSolverrRoundTripper(t *testing.T) {
	t.Parallel()

	// Mock FlareSolverr API server — returns a solved response.
	fsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if cmd, _ := req["cmd"].(string); cmd != "request.get" {
			t.Errorf("expected request.get, got %v", req["cmd"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"ok","message":"",
			"solution":{"url":"http://target/","status":200,
				"headers":{"X-Hello":"world"},
				"response":"<rss/>",
				"cookies":[{"name":"cf_clearance","value":"solved123"}],
				"userAgent":"FS-UA"}
		}`))
	}))
	defer fsSrv.Close()

	// Mock indexer site — returns CF challenge on first request,
	// then OK with solved cookies on retry.
	indexerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the request has a cf_clearance cookie (retry after solve).
		for _, c := range r.Cookies() {
			if c.Name == "cf_clearance" && c.Value == "solved123" {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte("<rss/>"))
				return
			}
		}
		// No clearance cookie — return CF challenge.
		w.Header().Set("Server", "cloudflare")
		w.WriteHeader(403)
		_, _ = w.Write([]byte("<title>Just a moment...</title>"))
	}))
	defer indexerSrv.Close()

	cli := proxies.NewFlareSolverrClient(nil, 30*time.Second)
	rt := cli.RoundTripperFor("p", proxies.FlareSolverrConfig{URL: fsSrv.URL})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, indexerSrv.URL+"/search", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<rss/>") {
		t.Errorf("body: %q", string(body))
	}

	// Verify UA was cached — second request should inject it.
	if ua, ok := cli.CachedUserAgent(req.URL.Hostname()); !ok || ua != "FS-UA" {
		t.Errorf("expected cached UA 'FS-UA', got %q (ok=%v)", ua, ok)
	}
}

func TestFlareSolverrRoundTripperNoCF(t *testing.T) {
	t.Parallel()

	// Mock indexer site — returns 200 directly (no CF).
	indexerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>results</html>"))
	}))
	defer indexerSrv.Close()

	cli := proxies.NewFlareSolverrClient(nil, 30*time.Second)
	rt := cli.RoundTripperFor("p", proxies.FlareSolverrConfig{URL: "http://unused"})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, indexerSrv.URL+"/search", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "results") {
		t.Errorf("body: %q", string(body))
	}
}

func TestFlareSolverrPing(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","message":"","sessions":[]}`))
	}))
	defer srv.Close()
	cli := proxies.NewFlareSolverrClient(nil, 30*time.Second)
	if err := cli.Ping(context.Background(), proxies.FlareSolverrConfig{URL: srv.URL}); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestFlareSolverrPingRejectsOversizedResponse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", 21<<20)))
	}))
	defer srv.Close()
	cli := proxies.NewFlareSolverrClient(nil, 30*time.Second)
	err := cli.Ping(context.Background(), proxies.FlareSolverrConfig{URL: srv.URL})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "response too large") {
		t.Fatalf("expected size limit error, got %v", err)
	}
}

func TestBuildHTTPTransport(t *testing.T) {
	t.Parallel()
	p := proxies.Proxy{
		ID: "p", Kind: proxies.KindHTTP, Name: "p",
		Enabled: true,
		Config:  json.RawMessage(`{"url":"http://proxy.example:8080","username":"u","password":"pp"}`),
	}
	rt, err := proxies.BuildTransport(p, nil)
	if err != nil {
		t.Fatalf("BuildTransport: %v", err)
	}
	if rt == nil {
		t.Fatal("nil transport")
	}
}

func TestBuildTransportRefusesDisabled(t *testing.T) {
	t.Parallel()
	p := proxies.Proxy{ID: "p", Kind: proxies.KindHTTP, Enabled: false,
		Config: json.RawMessage(`{"url":"http://x:8080"}`)}
	if _, err := proxies.BuildTransport(p, nil); err == nil {
		t.Fatal("expected error for disabled proxy")
	}
}
