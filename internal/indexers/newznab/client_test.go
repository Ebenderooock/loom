package newznab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/indexers"
)

// fixtureServer wires a httptest server that routes by `t=` and
// captures the last URL it served so tests can assert wire shape.
type fixtureServer struct {
	srv      *httptest.Server
	last     atomic.Value // url.Values of the last request
	handlers map[string]http.HandlerFunc
}

func newFixtureServer(t *testing.T) *fixtureServer {
	t.Helper()
	fs := &fixtureServer{handlers: map[string]http.HandlerFunc{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		fs.last.Store(r.URL.Query())
		mode := r.URL.Query().Get("t")
		h, ok := fs.handlers[mode]
		if !ok {
			http.Error(w, "unknown mode", http.StatusBadRequest)
			return
		}
		h(w, r)
	})
	fs.srv = httptest.NewServer(mux)
	t.Cleanup(fs.srv.Close)
	return fs
}

func (fs *fixtureServer) on(mode string, h http.HandlerFunc) { fs.handlers[mode] = h }
func (fs *fixtureServer) url() string                        { return fs.srv.URL + "/api" }
func (fs *fixtureServer) lastQuery() url.Values {
	v, _ := fs.last.Load().(url.Values)
	return v
}

func newTestClient(t *testing.T, urlStr string) *Client {
	t.Helper()
	cfg := Config{
		URL:         urlStr,
		APIKey:      "secret",
		UserAgent:   defaultUserAgent,
		Timeout:     durationString(2 * time.Second),
		attrFlavour: flavourNewznab,
	}
	return NewClient("ix-test", "Test", cfg, &http.Client{Timeout: 2 * time.Second}, nil)
}

func writeFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/xml")
	if _, err := w.Write(loadFixture(t, name)); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestClient_CapsTestSearchRoundtrip(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	fs.on("caps", func(w http.ResponseWriter, _ *http.Request) {
		writeFixture(t, w, "caps.xml")
	})
	fs.on("tvsearch", func(w http.ResponseWriter, _ *http.Request) {
		writeFixture(t, w, "tvsearch.xml")
	})

	c := newTestClient(t, fs.url())

	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}

	caps := c.Caps()
	if len(caps.SearchTypes) == 0 {
		t.Errorf("Caps after Test should be populated: %+v", caps)
	}

	res, err := c.Search(context.Background(), indexers.Query{Term: "show", TVDBID: "12345", Season: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Items) != 2 {
		t.Errorf("Items = %d, want 2", len(res.Items))
	}

	q := fs.lastQuery()
	if q.Get("t") != "tvsearch" {
		t.Errorf("upstream t = %q, want tvsearch", q.Get("t"))
	}
	if q.Get("apikey") != "secret" {
		t.Errorf("upstream apikey = %q", q.Get("apikey"))
	}
	if q.Get("tvdbid") != "12345" {
		t.Errorf("upstream tvdbid = %q", q.Get("tvdbid"))
	}
	if q.Get("season") != "2" {
		t.Errorf("upstream season = %q", q.Get("season"))
	}
}

func TestClient_HTTPErrorClassification(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"401 → ErrAuthFailed", http.StatusUnauthorized, "no", ErrAuthFailed},
		{"403 → ErrAuthFailed", http.StatusForbidden, "no", ErrAuthFailed},
		{"429 → ErrRateLimited", http.StatusTooManyRequests, "slow down", ErrRateLimited},
		{"500 → ErrUpstream", http.StatusInternalServerError, "boom", ErrUpstream},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fs := newFixtureServer(t)
			fs.on("caps", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			c := newTestClient(t, fs.url())
			err := c.Test(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want errors.Is %v", err, tc.want)
			}
		})
	}
}

func TestClient_UpstreamErrorEnvelope(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	fs.on("caps", func(w http.ResponseWriter, _ *http.Request) {
		writeFixture(t, w, "error_100.xml")
	})
	c := newTestClient(t, fs.url())
	err := c.Test(context.Background())
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
}

func TestClient_HTMLBodyMalformed(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	fs.on("caps", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><html>nope</html>"))
	})
	c := newTestClient(t, fs.url())
	err := c.Test(context.Background())
	if err == nil {
		t.Fatal("expected error on HTML body")
	}
	// HTML technically begins with '<' so we'll get a caps-parse
	// error wrapping the xml decode failure rather than ErrMalformedXML.
	if !errors.Is(err, ErrCapsParse) && !errors.Is(err, ErrMalformedXML) {
		t.Fatalf("err = %v, want ErrCapsParse or ErrMalformedXML", err)
	}
}

func TestClient_ContextCancel(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	fs.on("caps", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	})
	c := newTestClient(t, fs.url())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := c.Test(ctx)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// stubCapsCache is a thread-safe in-memory CapsCache for testing the
// preload + persist hooks.
type stubCapsCache struct {
	store map[string]indexers.Caps
}

func (s *stubCapsCache) Load(_ context.Context, id string) (indexers.Caps, bool, error) {
	c, ok := s.store[id]
	return c, ok, nil
}

func (s *stubCapsCache) Save(_ context.Context, id string, c indexers.Caps) error {
	if s.store == nil {
		s.store = map[string]indexers.Caps{}
	}
	s.store[id] = c
	return nil
}

func TestClient_CapsCachePreload(t *testing.T) {
	t.Parallel()
	stub := &stubCapsCache{store: map[string]indexers.Caps{
		"ix-test": {SearchTypes: []string{"search"}, Categories: []indexers.Category{2000}},
	}}
	cfg := Config{
		URL: "https://unused.example", APIKey: "x",
		UserAgent: defaultUserAgent, Timeout: durationString(time.Second),
		attrFlavour: flavourNewznab,
	}
	c := NewClient("ix-test", "Test", cfg, &http.Client{Timeout: time.Second}, stub)
	caps := c.Caps()
	if len(caps.SearchTypes) != 1 || caps.SearchTypes[0] != "search" {
		t.Fatalf("preload missed: %+v", caps)
	}
}

func TestClient_CapsCachePersistOnFetch(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	fs.on("caps", func(w http.ResponseWriter, _ *http.Request) {
		writeFixture(t, w, "caps.xml")
	})
	stub := &stubCapsCache{}
	cfg := Config{
		URL: fs.url(), APIKey: "secret",
		UserAgent: defaultUserAgent, Timeout: durationString(2 * time.Second),
		attrFlavour: flavourNewznab,
	}
	c := NewClient("ix-test", "Test", cfg, &http.Client{Timeout: 2 * time.Second}, stub)
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test: %v", err)
	}
	got, ok, _ := stub.Load(context.Background(), "ix-test")
	if !ok {
		t.Fatal("cache not persisted")
	}
	if len(got.SearchTypes) == 0 {
		t.Errorf("persisted caps empty: %+v", got)
	}
}

func TestParseConfig_RejectsEmpty(t *testing.T) {
	t.Parallel()
	_, err := parseConfig(nil)
	if err == nil {
		t.Fatal("expected error on empty config")
	}
}

func TestParseConfig_DefaultsAndExtractEmbeddedAPIKey(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"url":"https://feed.example/api/?apikey=abc"}`)
	cfg, err := parseConfig(raw)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.APIKey != "abc" {
		t.Errorf("APIKey = %q, want abc", cfg.APIKey)
	}
	if strings.HasSuffix(cfg.URL, "/") {
		t.Errorf("URL still has trailing slash: %q", cfg.URL)
	}
	if cfg.UserAgent == "" {
		t.Errorf("UserAgent default missing")
	}
	if cfg.Timeout.duration() == 0 {
		t.Errorf("Timeout default missing")
	}
}

func TestKindFactoriesRegistered(t *testing.T) {
	t.Parallel()
	for _, k := range []indexers.Kind{KindNewznab, KindTorznab} {
		if _, err := indexers.LookupKind(k); err != nil {
			t.Errorf("kind %s not registered: %v", k, err)
		}
	}
}

func TestKindFactory_BuildsClient(t *testing.T) {
	t.Parallel()
	fac, err := indexers.LookupKind(KindNewznab)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	def := indexers.Definition{
		ID:     "ix-1",
		Name:   "demo",
		Kind:   KindNewznab,
		Config: json.RawMessage(`{"url":"https://example.test/api","api_key":"k"}`),
	}
	ix, err := fac(context.Background(), def)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if ix.ID() != "ix-1" {
		t.Errorf("ID = %q", ix.ID())
	}
	if _, ok := ix.(*Client); !ok {
		t.Errorf("factory returned %T, want *Client", ix)
	}
}

// guard against accidental log spam on err paths
var _ = fmt.Sprintf
