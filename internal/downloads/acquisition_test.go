package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/workflows"
)

type acquisitionClient struct {
	DownloadClient
	calls atomic.Int32
	fail  bool
	err   error
}

func (c *acquisitionClient) ID() string { return "client" }
func (c *acquisitionClient) Add(context.Context, AddRequest) (AddResult, error) {
	c.calls.Add(1)
	if c.err != nil {
		return AddResult{}, c.err
	}
	if c.fail {
		return AddResult{}, errors.New("https://user:password@tracker.invalid?apikey=secret provider response")
	}
	return AddResult{ClientID: "client", ItemID: "download"}, nil
}

func acquisitionRegistry(t *testing.T, store *HistoryStore, fail bool) (*Registry, *acquisitionClient) {
	t.Helper()
	r := NewRegistry()
	c := &acquisitionClient{fail: fail}
	if err := r.Register(c); err != nil {
		t.Fatal(err)
	}
	r.SetHistoryStore(store)
	return r, c
}

func TestAcquisitionBoundedAcrossRestartsAndRecovery(t *testing.T) {
	ctx := context.Background()
	store := openHistoryTestStore(t)
	req := AddRequest{Title: "Release", TorrentURL: "https://tracker.invalid?apikey=secret", IndexerID: "indexer", MediaType: "movie", MediaID: "movie"}
	for attempt := 1; attempt <= maxAcquisitionAttempts; attempt++ {
		r, client := acquisitionRegistry(t, NewHistoryStore(store.db), true)
		_, err := r.Acquire(ctx, "client", req, nil)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("expected sanitized failure, got %v", err)
		}
		if _, err = r.Acquire(ctx, "client", req, nil); !errors.Is(err, ErrAcquisitionDeferred) {
			t.Fatalf("expected deferred duplicate: %v", err)
		}
		if client.calls.Load() != 1 {
			t.Fatal("duplicate client call")
		}
		entries, err := store.List(ctx, 50, 0)
		if err != nil || len(entries) != attempt {
			t.Fatalf("history: %v, %d entries", err, len(entries))
		}
		var deadline string
		var count int
		if err := store.db.QueryRow(`SELECT next_retry_at, retry_count FROM download_history ORDER BY attempt DESC LIMIT 1`).Scan(&deadline, &count); err != nil {
			t.Fatal(err)
		}
		if count != attempt {
			t.Fatalf("persistent retry count: got %d, want %d", count, attempt)
		}
		if attempt < maxAcquisitionAttempts {
			at, err := time.Parse(time.RFC3339, deadline)
			if err != nil || time.Until(at) < workflows.RetryBackoff(attempt)-5*time.Second ||
				time.Until(at) > workflows.RetryBackoff(attempt) {
				t.Fatalf("incorrect retry deadline %q: %v", deadline, err)
			}
		}
		// Advance the persisted deadline without resetting the budget.
		if _, err := store.db.Exec(`UPDATE download_history SET next_retry_at = ''`); err != nil {
			t.Fatal(err)
		}

	}
	r, client := acquisitionRegistry(t, NewHistoryStore(store.db), false)
	if _, err := r.Acquire(ctx, "client", req, nil); !errors.Is(err, ErrAcquisitionDeferred) {
		t.Fatalf("exhausted release admitted: %v", err)
	}
	if err := store.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Acquire(ctx, "client", req, nil); !errors.Is(err, ErrAcquisitionDeferred) {
		t.Fatalf("clear erased retry budget: %v", err)
	}
	var id string
	if err := store.db.QueryRow(`SELECT id FROM download_history ORDER BY attempt DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := store.acquisitionAction(ctx, id, "retry"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Acquire(ctx, "client", req, nil); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	if _, err := r.Acquire(ctx, "client", req, nil); !errors.Is(err, ErrAcquisitionDeferred) {
		t.Fatalf("successful release duplicated: %v", err)
	}
	if client.calls.Load() != 1 {
		t.Fatal("recovery should add once")
	}
}

func TestAcquisitionCancellationKeepsAmbiguousAdmission(t *testing.T) {
	store := openHistoryTestStore(t)
	r, client := acquisitionRegistry(t, store, false)
	client.err = context.DeadlineExceeded
	req := AddRequest{Title: "Ambiguous", TorrentURL: "https://tracker.invalid/torrent"}
	if _, err := r.Acquire(context.Background(), "client", req, nil); err == nil {
		t.Fatal("expected unknown outcome")
	}
	entries, err := store.List(context.Background(), 50, 0)
	if err != nil || len(entries) != 1 || entries[0].Status != "acquiring" {
		t.Fatalf("ambiguous outcome was made retryable: %+v, %v", entries, err)
	}
	if err := store.acquisitionAction(context.Background(), entries[0].ID, "retry"); !errors.Is(err, ErrAcquisitionDeferred) {
		t.Fatalf("active admission was reset: %v", err)
	}
	if _, err := r.Acquire(context.Background(), "client", req, nil); !errors.Is(err, ErrAcquisitionDeferred) {
		t.Fatalf("ambiguous request repeated: %v", err)
	}
}

type acquisitionIndexer struct {
	indexers.Indexer
	id    string
	fail  bool
	calls atomic.Int32
}

func (p *acquisitionIndexer) ID() string          { return p.id }
func (p *acquisitionIndexer) Name() string        { return p.id }
func (p *acquisitionIndexer) Caps() indexers.Caps { return indexers.Caps{} }
func (p *acquisitionIndexer) Search(context.Context, indexers.Query) (*indexers.Results, error) {
	p.calls.Add(1)
	if p.fail {
		return nil, errors.New("provider response with secret")
	}
	return &indexers.Results{}, nil
}

func TestAcquisitionProviderSearchBudgetsAndRecovery(t *testing.T) {
	store := openHistoryTestStore(t)
	ctx := context.Background()
	r, _ := acquisitionRegistry(t, store, false)
	broken := &acquisitionIndexer{id: "broken", fail: true}
	healthy := &acquisitionIndexer{id: "healthy"}
	registry := indexers.NewRegistry()
	for _, p := range []*acquisitionIndexer{broken, healthy} {
		if err := registry.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	newService := func() *indexers.Service {
		svc, err := indexers.NewService(indexers.ServiceOptions{
			Registry: registry, Repository: indexers.NewSQLiteRepository(store.db),
		})
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}
	q := indexers.Query{Term: "Movie"}
	media := AddRequest{Title: "Movie", MediaID: "movie", MediaType: "movie"}
	for range maxAcquisitionAttempts {
		if _, err := r.SearchAcquisition(ctx, newService(), q, media); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`UPDATE download_history SET next_retry_at = ''`); err != nil {
			t.Fatal(err)
		}
	}
	restarted, _ := acquisitionRegistry(t, NewHistoryStore(store.db), false)
	if _, err := restarted.SearchAcquisition(ctx, newService(), q, media); err != nil {
		t.Fatal(err)
	}
	if broken.calls.Load() != 3 || healthy.calls.Load() != 4 {
		t.Fatalf("provider isolation/budget failed: broken=%d healthy=%d", broken.calls.Load(), healthy.calls.Load())
	}
	entries, err := store.List(ctx, 50, 0)
	if err != nil || len(entries) != 3 {
		t.Fatalf("failure history: %+v, %v", entries, err)
	}
	var latest string
	for _, entry := range entries {
		if entry.IsLatest {
			latest = entry.ID
		}
		if entry.Stage != "provider_search" || strings.Contains(entry.Reason, "secret") || entry.IndexerID != "broken" {
			t.Fatalf("provider history: %+v", entry)
		}
	}
	if err := store.acquisitionAction(ctx, latest, "retry"); err != nil {
		t.Fatal(err)
	}
	broken.fail = false
	if _, err := restarted.SearchAcquisition(ctx, newService(), q, media); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.SearchAcquisition(ctx, newService(), q, media); err != nil {
		t.Fatal(err)
	}
	if broken.calls.Load() != 5 {
		t.Fatal("successful recovery did not reset provider failure budget")
	}
	// All-denied quota filtering must not consume persistent attempts.
	denied := indexers.WithSearchAdmission(ctx, func(string) bool { return false })
	if _, err := restarted.SearchAcquisition(denied, newService(), q, media); err != nil {
		t.Fatal(err)
	}
	if broken.calls.Load() != 5 {
		t.Fatal("quota denial called provider")
	}
}

func TestAcquisitionIdentityNormalizesMagnetsAndSignedLinks(t *testing.T) {
	hash := strings.Repeat("ab", 20)
	first, _ := acquisitionIdentity(AddRequest{Infohash: hash})
	second, _ := acquisitionIdentity(AddRequest{Magnet: "magnet:?xt=urn:btih:" + strings.ToUpper(hash) + "&tr=secret"})
	if first != second {
		t.Fatal("magnet/infohash identity differs")
	}
	first, _ = acquisitionIdentity(AddRequest{Title: "Release", IndexerID: "tracker", TorrentURL: "https://tracker.invalid?token=one"})
	second, _ = acquisitionIdentity(AddRequest{Title: "Release", IndexerID: "tracker", TorrentURL: "https://tracker.invalid?token=two"})
	if first != second {
		t.Fatal("signed link reset release identity")
	}
}
func TestAcquisitionProviderAndPermanentFailures(t *testing.T) {
	for _, stage := range []string{"provider", "validation", "client"} {
		t.Run(stage, func(t *testing.T) {
			store := openHistoryTestStore(t)
			r, client := acquisitionRegistry(t, store, true)
			req := AddRequest{Title: "Release", IndexerID: "indexer"}
			if stage != "validation" {
				req.TorrentURL = "https://tracker.invalid?apikey=secret"
			}
			var prepare func(*AddRequest) error
			if stage == "provider" {
				prepare = func(*AddRequest) error { return errors.New("provider secret") }
			}
			if _, err := r.Acquire(context.Background(), "client", req, prepare); err == nil {
				t.Fatal("expected failure")
			}
			entries, err := store.List(context.Background(), 50, 0)
			if err != nil || len(entries) != 1 {
				t.Fatalf("history: %v", err)
			}
			entry := entries[0]
			if entry.Stage != stage || entry.RetryEligible != (stage != "validation") ||
				strings.Contains(entry.Reason, "secret") {
				t.Fatalf("invalid classification: %+v", entry)
			}
			if stage != "client" && client.calls.Load() != 0 {
				t.Fatal("client called before successful preparation")
			}
			blocked, err := NewBlocklistStore(store.db).IsBlocked(context.Background(), entry.ReleaseHash)
			if err != nil || blocked {
				t.Fatalf("failure automatically blocklisted release: %v", err)
			}
		})
	}
}

func TestAcquisitionConcurrentAndInterruptedAdmission(t *testing.T) {
	store := openHistoryTestStore(t)
	ctx := context.Background()
	req := AddRequest{Title: "Release", TorrentURL: "https://tracker.invalid/release"}
	r, client := acquisitionRegistry(t, store, false)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := r.Acquire(ctx, "client", req, nil)
			if err != nil && !errors.Is(err, ErrAcquisitionDeferred) {
				t.Errorf("admission: %v", err)
			}
		})
	}
	wg.Wait()
	if client.calls.Load() != 1 {
		t.Fatalf("added %d times", client.calls.Load())
	}
	req.Title = "Interrupted"
	id, err := store.beginAcquisition(ctx, req, "client", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	restarted, _ := acquisitionRegistry(t, NewHistoryStore(store.db), false)
	if _, err := restarted.Acquire(ctx, "client", req, nil); !errors.Is(err, ErrAcquisitionDeferred) {
		t.Fatalf("interrupted admission retried: %v", err)
	}
	if err := store.acquisitionAction(ctx, id, "blocklist"); err != nil {
		t.Fatal(err)
	}
	if err := store.acquisitionAction(ctx, id, "retry"); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Acquire(ctx, "client", req, nil); !errors.Is(err, ErrAcquisitionDeferred) {
		t.Fatalf("retry bypassed blocklist: %v", err)
	}
}

func TestAcquisitionActionsAuthorizationAndMixedBatch(t *testing.T) {
	store := openHistoryTestStore(t)
	ctx := context.Background()
	id, err := store.beginAcquisition(ctx, AddRequest{Title: "Release"}, "client", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{historyStore: store}
	for _, roles := range [][]string{nil, {"user"}, {"admin"}} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/downloads/history/actions",
			strings.NewReader(`{"ids":["`+id+`","missing"],"action":"retry"}`))
		if roles != nil {
			request = request.WithContext(auth.WithIdentity(ctx, &auth.Identity{Roles: roles}))
		}
		response := httptest.NewRecorder()
		s.handleAcquisitionActions(response, request)
		if len(roles) == 0 || roles[0] != "admin" {
			if response.Code != http.StatusForbidden {
				t.Fatalf("unauthorized action: %d", response.Code)
			}
			continue
		}
		var body struct {
			Results []struct {
				OK bool `json:"ok"`
			} `json:"results"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Results) != 2 ||
			!body.Results[0].OK || body.Results[1].OK {
			t.Fatalf("mixed results: %s, %v", response.Body.String(), err)
		}
	}
}
