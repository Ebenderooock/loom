package nzbget

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
)

func TestClient_VersionProbe(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "user", "pw")
	defer f.close()
	f.on("version", "21.1")

	c := newTestClient(t, f)
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if got := f.callCount("version"); got != 1 {
		t.Fatalf("expected one version call, got %d", got)
	}
}

func TestClient_VersionProbeEmpty(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "user", "pw")
	defer f.close()
	f.on("version", "")

	c := newTestClient(t, f)
	err := c.Test(context.Background())
	if !errors.Is(err, ErrServer) {
		t.Fatalf("expected ErrServer, got %v", err)
	}
}

func TestClient_BasicAuthRejection(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "user", "pw")
	defer f.close()

	cfg := Config{}
	u, _ := url.Parse(f.srv.URL)
	cfg.Host = u.Hostname()
	cfg.Port, _ = strconv.Atoi(u.Port())
	cfg.BasePath = "/"
	cfg.Username = "user"
	cfg.Password = "wrong"
	c := NewClient("bad", "Bad", cfg, f.srv.Client())

	err := c.Test(context.Background())
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("expected ErrAuth, got %v", err)
	}
}

func TestClient_BasicAuthHeaderSent(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "alice", "s3cret")
	defer f.close()
	f.on("version", "22.0")

	c := newTestClient(t, f)
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
	// implicitly verified by fake's BasicAuth check; the call would
	// have surfaced ErrAuth otherwise.
}

func TestClient_HTTP500MapsToUpstream(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.onFunc("version", func(_ *testing.T, _ []any) (any, *rpcError, int) {
		return nil, nil, http.StatusInternalServerError
	})

	c := newTestClient(t, f)
	err := c.Test(context.Background())
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("expected ErrUpstream, got %v", err)
	}
}

func TestClient_RPCErrorEnvelope(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.onErr("version", &rpcError{Code: -32601, Message: "Method not found"})

	c := newTestClient(t, f)
	err := c.Test(context.Background())
	if !errors.Is(err, ErrServer) {
		t.Fatalf("expected ErrServer, got %v", err)
	}
	if !strings.Contains(err.Error(), "Method not found") {
		t.Fatalf("error should include upstream message, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "-32601") {
		t.Fatalf("error should include rpc code, got %q", err.Error())
	}
}

func TestClient_BasePathHonored(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	hit := false
	mux.HandleFunc("/nzb/jsonrpc", func(w http.ResponseWriter, r *http.Request) {
		if user, pass, _ := r.BasicAuth(); user != "u" || pass != "p" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		hit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"21.1"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := Config{Host: u.Hostname(), Port: port, BasePath: "/nzb", Username: "u", Password: "p"}
	c := NewClient("id", "n", cfg, srv.Client())
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !hit {
		t.Fatal("expected server to receive request at /nzb/jsonrpc")
	}
}

func TestClient_TLSScheme(t *testing.T) {
	t.Parallel()
	cfg := Config{Host: "h.example", Port: 6791, TLS: true, BasePath: "/"}
	c := NewClient("x", "x", cfg, nil)
	got := c.rpcEndpoint()
	if !strings.HasPrefix(got, "https://") {
		t.Fatalf("expected https scheme, got %q", got)
	}
	if !strings.HasSuffix(got, "/jsonrpc") {
		t.Fatalf("expected /jsonrpc suffix, got %q", got)
	}
}

func TestClient_RawJSONRPCEnvelope(t *testing.T) {
	t.Parallel()
	// End-to-end: ensure the request body shape matches NZBGet's
	// expectation — jsonrpc=2.0, method, params, id.
	var captured rpcRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"21.1"}`)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := Config{Host: u.Hostname(), Port: port, BasePath: "/", Username: "u", Password: "p"}
	c := NewClient("x", "x", cfg, srv.Client())
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if captured.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc = %q want 2.0", captured.JSONRPC)
	}
	if captured.Method != "version" {
		t.Fatalf("method = %q", captured.Method)
	}
	if captured.ID == 0 {
		t.Fatalf("id should be non-zero, got %d", captured.ID)
	}
}
