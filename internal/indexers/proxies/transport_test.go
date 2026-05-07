package proxies_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ebenderooock/loom/internal/indexers/proxies"
)

// TestProviderRoutesThroughHTTPProxy spins up an httptest server that
// behaves as a forward HTTP proxy (it serves the absolute-URI path
// directly) and verifies that a Provider built around an HTTP proxy
// row directs traffic through it.
func TestProviderRoutesThroughHTTPProxy(t *testing.T) {
	t.Parallel()

	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("hello"))
	}))
	defer upstream.Close()

	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Standard library's http.ProxyURL forwards as
		// absolute-URI GET to the proxy. Forward by issuing a
		// fresh client request.
		req, err := http.NewRequest(r.Method, r.RequestURI, r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer proxySrv.Close()

	_, raw := openTestDB(t)
	repo := proxies.NewSQLiteRepository(raw)
	if _, err := repo.Create(context.Background(), proxies.Proxy{
		ID: "p", Kind: proxies.KindHTTP, Name: "p", Enabled: true,
		Config: json.RawMessage(`{"url":"` + proxySrv.URL + `"}`),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	prov := proxies.NewProvider(repo, nil)
	rt, err := prov.TransportFor("p")
	if err != nil {
		t.Fatalf("TransportFor: %v", err)
	}
	cli := &http.Client{Transport: rt}
	resp, err := cli.Get(upstream.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if hits != 1 {
		t.Fatalf("upstream hits: %d (expected 1)", hits)
	}

	// Empty proxy ID returns DefaultTransport, which doesn't go
	// through proxySrv.
	rt2, err := prov.TransportFor("")
	if err != nil || rt2 != http.DefaultTransport {
		t.Fatalf("expected DefaultTransport for empty id, got %v err=%v", rt2, err)
	}

	// Invalidate is idempotent.
	prov.Invalidate("p")
	prov.Invalidate("")
}
