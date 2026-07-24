package newznab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/indexers"
)

type testHTTPClientProvider struct {
	clients map[string]*http.Client
}

func (p testHTTPClientProvider) HTTPClientFor(def indexers.Definition, _ time.Duration) *http.Client {
	return p.clients[def.ID]
}

func TestFactoryUsesInjectedHTTPClientPerDefinition(t *testing.T) {
	fixture := loadFixture(t, "caps.xml")
	var hitsA int32
	var hitsB int32

	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hitsA, 1)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(serverA.Close)

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hitsB, 1)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(serverB.Close)

	restore := indexers.SwapHTTPClientProvider(testHTTPClientProvider{
		clients: map[string]*http.Client{
			"ix-a": serverA.Client(),
			"ix-b": serverB.Client(),
		},
	})
	t.Cleanup(restore)

	factory := factoryFor(flavourNewznab)

	build := func(id, baseURL string) indexers.Indexer {
		t.Helper()
		def := indexers.Definition{
			ID:   id,
			Kind: KindNewznab,
			Name: id,
			Config: []byte(fmt.Sprintf(`{
				"url":"%s/api",
				"api_key":"secret"
			}`, baseURL)),
		}
		ix, err := factory(context.Background(), def)
		if err != nil {
			t.Fatalf("factory build %s: %v", id, err)
		}
		return ix
	}

	ixA := build("ix-a", serverA.URL)
	ixB := build("ix-b", serverB.URL)

	if err := ixA.Test(context.Background()); err != nil {
		t.Fatalf("ixA test: %v", err)
	}
	if err := ixB.Test(context.Background()); err != nil {
		t.Fatalf("ixB test: %v", err)
	}

	if got := atomic.LoadInt32(&hitsA); got != 1 {
		t.Fatalf("serverA hits = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&hitsB); got != 1 {
		t.Fatalf("serverB hits = %d, want 1", got)
	}
}
