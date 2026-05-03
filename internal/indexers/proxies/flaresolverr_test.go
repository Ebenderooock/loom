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

	"github.com/loomctl/loom/internal/indexers/proxies"
)

func TestFlareSolverrRoundTripper(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				"cookies":[{"name":"a","value":"b"}],
				"userAgent":"FS-UA"}
		}`))
	}))
	defer srv.Close()

	cli := proxies.NewFlareSolverrClient(nil, 30*time.Second)
	rt := cli.RoundTripperFor("p", proxies.FlareSolverrConfig{URL: srv.URL})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://target/", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Hello"); got != "world" {
		t.Errorf("header: %q", got)
	}
	if got := resp.Header.Get("User-Agent"); got != "FS-UA" {
		t.Errorf("UA header: %q", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<rss/>") {
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
