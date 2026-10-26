package indexers_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/indexers"
	_ "github.com/ebenderooock/loom/internal/indexers/newznab"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

func budgetFixture(t *testing.T, clock indexers.Clock, allowance int64) (*indexers.ProviderBudgetStore, *indexers.Registry) {
	t.Helper()
	_, db := openTestDB(t)
	repo := indexers.NewSQLiteRepository(db)
	r := indexers.NewRegistry()
	for _, id := range []string{"one", "two"} {
		_, err := repo.Create(context.Background(), indexers.Definition{ID: id, Kind: indexers.KindNull, Name: id})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Register(&fakeIndexer{id: id, name: id, searchErr: errors.New("provider failure")}); err != nil {
			t.Fatal(err)
		}
	}
	s := indexers.NewProviderBudgetStore(db, clock)
	if err := s.Configure(context.Background(), indexers.ProviderBudget{
		ID: "account-a", Allowance: allowance, WindowSeconds: 60, IndexerIDs: []string{"one", "two"},
	}); err != nil {
		t.Fatal(err)
	}
	return s, r
}

func TestProviderBudgetConcurrentSharedQueriesAndReset(t *testing.T) {
	clock := &fixedClock{now: time.Now()}
	s, r := budgetFixture(t, clock, 5)
	var wg sync.WaitGroup
	var attempts atomic.Int64
	for i := range 50 {
		wg.Go(func() {
			id := []string{"one", "two"}[i%2]
			q := []indexers.Query{{IMDBID: "tt123"}, {Term: "Title"}, {Term: "Alternate title"}}[i%3]
			out := r.Search(context.Background(), q, indexers.SearchOptions{
				IndexerIDs: []string{id}, ProviderBudgets: s,
			})
			if len(out.Errors) > 0 {
				attempts.Add(1)
			} else if out.Diagnostics.Indexers[0].Status != "skipped" || out.Diagnostics.Indexers[0].ResumeAt == 0 {
				t.Errorf("missing budget skip/reset reason: %+v", out)
			}
		})
	}
	wg.Wait()
	if attempts.Load() != 5 {
		t.Fatalf("admitted %d, want 5 including failed attempts", attempts.Load())
	}
	list, err := s.List(context.Background())
	if err != nil || list[0].Consumed != 5 {
		t.Fatalf("persisted allowance: %+v %v", list, err)
	}
	clock.now = time.Unix(list[0].ResetAt, 0)
	if err := s.Reserve(context.Background(), "two"); err != nil {
		t.Fatalf("exact reset boundary: %v", err)
	}
	list, _ = s.List(context.Background())
	if list[0].Consumed != 1 {
		t.Fatalf("reset must consume first new attempt: %+v", list[0])
	}
}

func TestProviderBudgetSurvivesActualDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "restart.db")
	cfg := config.StorageConfig{Engine: "sqlite", SQLite: config.SQLiteConfig{Path: path}}
	db, err := storage.Open(ctx, cfg, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	repo := indexers.NewSQLiteRepository(db.DB())
	if _, err := repo.Create(ctx, indexers.Definition{ID: "one", Kind: indexers.KindNull, Name: "one"}); err != nil {
		t.Fatal(err)
	}
	s := indexers.NewProviderBudgetStore(db.DB(), nil)
	if err := s.Configure(ctx, indexers.ProviderBudget{ID: "shared", Allowance: 1, WindowSeconds: 86400, IndexerIDs: []string{"one"}}); err != nil {
		t.Fatal(err)
	}
	r := indexers.NewRegistry()
	installBudgetHTTP(t, r, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Unix()+86400, 10))
	})
	r.Search(ctx, indexers.Query{Term: "search"}, indexers.SearchOptions{
		IndexerIDs: []string{"one"}, ProviderBudgets: s,
	})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.Open(ctx, cfg, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = indexers.NewProviderBudgetStore(db.DB(), nil)
	budgets, err := s.List(ctx)
	if err != nil || budgets[0].ProviderRemaining == nil || *budgets[0].ProviderRemaining != 0 || budgets[0].ProviderResetAt == 0 {
		t.Fatalf("restart cleared provider state: %+v %v", budgets, err)
	}
	var denied *indexers.BudgetDenied
	if err := s.Reserve(ctx, "one"); !errors.As(err, &denied) {
		t.Fatalf("restart cleared allowance: %v", err)
	}
	// Editing a group must not reset already consumed calls.
	if err := s.Configure(ctx, indexers.ProviderBudget{ID: "shared", Allowance: 1, WindowSeconds: 60, IndexerIDs: []string{"one"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Reserve(ctx, "one"); !errors.As(err, &denied) {
		t.Fatalf("configuration cleared allowance: %v", err)
	}
}

type budgetHTTPIndexer struct {
	id     string
	url    string
	client *http.Client
}

func (p *budgetHTTPIndexer) ID() string                 { return p.id }
func (p *budgetHTTPIndexer) Name() string               { return p.id }
func (p *budgetHTTPIndexer) Caps() indexers.Caps        { return indexers.Caps{} }
func (p *budgetHTTPIndexer) Test(context.Context) error { return nil }
func (p *budgetHTTPIndexer) Search(ctx context.Context, _ indexers.Query) (*indexers.Results, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return &indexers.Results{}, nil
}

func installBudgetHTTP(t *testing.T, r *indexers.Registry, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	for _, id := range []string{"one", "two"} {
		transport, err := indexers.TransportForDefinition(indexers.Definition{ID: id, Kind: "newznab"})
		if err != nil {
			t.Fatal(err)
		}
		client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
		if err := r.Replace(&budgetHTTPIndexer{id: id, url: server.URL, client: client}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProviderBudgetSignalsAndTransportRetries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		remaining string
		reset     string
	}{
		{"remaining zero", 200, "0", "valid"},
		{"429 retry after", 429, "", "retry"},
		{"unknown reset", 200, "0", ""},
		{"invalid reset", 200, "0", "-42"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clock := &fixedClock{now: time.Now()}
			s, r := budgetFixture(t, clock, 100)
			var calls atomic.Int64
			installBudgetHTTP(t, r, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if tc.remaining != "" {
					w.Header().Set("X-RateLimit-Remaining", tc.remaining)
				}
				switch tc.reset {
				case "valid":
					w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(clock.now.Unix()+120, 10))
				case "retry":
					w.Header().Set("Retry-After", "1")
				default:
					w.Header().Set("X-RateLimit-Reset", tc.reset)
				}
				w.WriteHeader(tc.status)
			})
			opts := indexers.SearchOptions{IndexerIDs: []string{"one"}, ProviderBudgets: s}
			r.Search(context.Background(), indexers.Query{Term: "manual"}, opts)
			opts.IndexerIDs = []string{"two"}
			out := r.Search(context.Background(), indexers.Query{Term: "alternate"}, opts)
			if calls.Load() != 1 || out.Diagnostics.Indexers[0].Status != "skipped" {
				t.Fatalf("provider signal ignored: calls=%d out=%+v", calls.Load(), out)
			}
			list, err := s.List(context.Background())
			if err != nil || list[0].ProviderRemaining == nil || *list[0].ProviderRemaining != 0 {
				t.Fatalf("provider signal not persisted: %+v %v", list, err)
			}
			if tc.reset == "valid" || tc.reset == "retry" {
				clock.now = time.Unix(list[0].ProviderResetAt, 0)
				if err := s.Reserve(context.Background(), "two"); err != nil {
					t.Fatalf("provider reset boundary: %v", err)
				}
			} else {
				clock.now = clock.now.Add(48 * time.Hour)
				if err := s.Reserve(context.Background(), "two"); err == nil {
					t.Fatal("unknown provider reset must fail closed after local reset")
				}
			}
		})
	}
}

func TestProviderBudgetManualStreamingAndEligibility(t *testing.T) {
	s, r := budgetFixture(t, nil, 1)
	p := &fakeIndexer{id: "one", name: "one", caps: indexers.Caps{SupportedIDs: []string{"tvdb"}}}
	if err := r.Replace(p); err != nil {
		t.Fatal(err)
	}
	opts := indexers.SearchOptions{IndexerIDs: []string{"one"}, ProviderBudgets: s}
	r.Search(context.Background(), indexers.Query{IMDBID: "tt123"}, opts)
	r.Search(context.Background(), indexers.Query{Term: "manual"}, opts)
	events := make(chan indexers.StreamEvent, 10)
	r.SearchStream(context.Background(), indexers.Query{Term: "manual stream"}, opts, events)
	var skipped bool
	for event := range events {
		if event.Status == "skipped" && event.ResumeAt > 0 {
			skipped = true
		}
	}
	if p.calls.Load() != 1 || !skipped {
		t.Fatalf("manual/stream admission mismatch: calls=%d skipped=%v", p.calls.Load(), skipped)
	}
}

func TestProviderBudgetAPIAdminAndAccountingProtection(t *testing.T) {
	s, _ := budgetFixture(t, nil, 3)
	_, db := openTestDB(t)
	service, err := indexers.NewService(indexers.ServiceOptions{
		Repository: indexers.NewSQLiteRepository(db), ProviderBudgets: s,
	})
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	service.Mount(router)
	for _, tc := range []struct {
		method, path, body string
		role               string
		status             int
	}{
		{"GET", "/api/v1/indexers/provider-budgets", "", "viewer", 403},
		{"PUT", "/api/v1/indexers/provider-budgets/account-a", `{}`, "viewer", 403},
		{"GET", "/api/v1/indexers/provider-budgets", "", "admin", 200},
		{"PUT", "/api/v1/indexers/provider-budgets/account-a", `{"allowance":10,"window_seconds":60,"consumed":0}`, "admin", 400},
		{"PUT", "/api/v1/indexers/provider-budgets/account-a", `{"allowance":10,"window_seconds":60,"indexer_ids":["one","two"]}`, "admin", 204},
	} {
		t.Run(fmt.Sprintf("%s-%s-%d", tc.method, tc.role, tc.status), func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req = req.WithContext(auth.WithIdentity(req.Context(), &auth.Identity{Roles: []string{tc.role}}))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
		})
	}
}

func TestProviderBudgetRetryAttemptsConsumeAllowance(t *testing.T) {
	s, r := budgetFixture(t, nil, 2)
	var calls atomic.Int64
	installBudgetHTTP(t, r, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	out := r.Search(context.Background(), indexers.Query{Term: "test"}, indexers.SearchOptions{
		IndexerIDs: []string{"one"}, ProviderBudgets: s,
	})
	if calls.Load() != 2 || out.Diagnostics.Indexers[0].Status != "skipped" {
		t.Fatalf("retry exceeded allowance: calls=%d out=%+v", calls.Load(), out)
	}
}

func TestProviderBudgetProtocolExhaustion(t *testing.T) {
	for _, knownReset := range []bool{false, true} {
		t.Run(fmt.Sprintf("known-reset-%v", knownReset), func(t *testing.T) {
			s, r := budgetFixture(t, nil, 100)
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Query().Get("t") == "caps" {
					_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/></searching></caps>`))
					return
				}
				calls.Add(1)
				if knownReset {
					w.Header().Set("X-RateLimit-Remaining", "0")
					w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Unix()+120, 10))
				}
				_, _ = w.Write([]byte(`<error code="100" description="API limit reached"/>`))
			}))
			defer server.Close()
			factory, err := indexers.LookupKind("newznab")
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(map[string]string{"url": server.URL, "api_key": "private-test-key"})
			if err != nil {
				t.Fatal(err)
			}
			ix, err := factory(context.Background(), indexers.Definition{ID: "one", Name: "one", Config: raw})
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Replace(ix); err != nil {
				t.Fatal(err)
			}
			opts := indexers.SearchOptions{IndexerIDs: []string{"one"}, ProviderBudgets: s}
			r.Search(context.Background(), indexers.Query{Term: "title"}, opts)
			out := r.Search(context.Background(), indexers.Query{Term: "alternate"}, opts)
			if calls.Load() != 1 || out.Diagnostics.Indexers[0].Status != "skipped" ||
				strings.Contains(out.Diagnostics.Indexers[0].ErrorMessage, "private-test-key") {
				t.Fatalf("protocol exhaustion not shared/persisted safely: calls=%d out=%+v", calls.Load(), out)
			}
			if knownReset && out.Diagnostics.Indexers[0].ResumeAt == 0 {
				t.Fatal("XML signal erased a trustworthy HTTP reset")
			}
		})
	}
}

func TestProviderBudgetMembershipAndCancellation(t *testing.T) {
	s, r := budgetFixture(t, nil, 3)
	ctx := context.Background()
	// Same hostname, different account group: only explicit membership matters.
	if err := s.Configure(ctx, indexers.ProviderBudget{ID: "other-account", Allowance: 5, WindowSeconds: 60, IndexerIDs: []string{"one"}}); err == nil {
		t.Fatal("silently reassigned an existing group member")
	}

	groups, err := s.List(ctx)
	if err != nil || len(groups) != 1 || len(groups[0].IndexerIDs) != 2 {
		t.Fatalf("failed configuration was not atomic: %+v %v", groups, err)
	}
	if err := s.Reserve(ctx, "unconfigured"); err != nil {
		t.Fatalf("unconfigured account inherited another group's allowance: %v", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	r.Search(ctx, indexers.Query{Term: "cancelled"}, indexers.SearchOptions{
		IndexerIDs: []string{"one"}, ProviderBudgets: s,
	})
	groups, err = s.List(context.Background())
	if err != nil || groups[0].Consumed != 0 {
		t.Fatalf("cancelled query consumed allowance: %+v %v", groups, err)
	}
}

func TestProviderBudgetPostgres(t *testing.T) {
	dsn := os.Getenv("LOOM_TEST_PROVIDER_BUDGET_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set LOOM_TEST_PROVIDER_BUDGET_POSTGRES_DSN to test PostgreSQL")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Temporary tables keep this opt-in check isolated from existing data.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TEMP TABLE indexers (id TEXT PRIMARY KEY); INSERT INTO indexers VALUES ('one'),('two')`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../storage/migrations/postgres/0085_provider_budgets.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(migration), "-- +goose Down")[0]
	up = strings.ReplaceAll(up, "CREATE TABLE", "CREATE TEMP TABLE")
	if _, err := db.Exec(up); err != nil {
		t.Fatal(err)
	}
	clock := &fixedClock{now: time.Now()}
	s := indexers.NewProviderBudgetStore(db, clock)
	if err := s.Configure(context.Background(), indexers.ProviderBudget{ID: "pg-account", Allowance: 5, WindowSeconds: 60, IndexerIDs: []string{"one", "two"}}); err != nil {
		t.Fatal(err)
	}
	r := indexers.NewRegistry()
	installBudgetHTTP(t, r, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("RateLimit-Remaining", "0")
		w.Header().Set("RateLimit-Reset", "60")
	})
	out := r.Search(context.Background(), indexers.Query{Term: "query"}, indexers.SearchOptions{
		IndexerIDs: []string{"one"}, ProviderBudgets: s,
	})
	if out.Diagnostics.Indexers[0].Status != "ok" {
		t.Fatalf("PostgreSQL accounting failed: %+v", out)
	}
	if err := s.Reserve(context.Background(), "two"); err == nil {
		t.Fatal("PostgreSQL provider signal ignored")
	}
	budgets, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	clock.now = time.Unix(budgets[0].ProviderResetAt, 0)
	if err := s.Reserve(context.Background(), "two"); err != nil {
		t.Fatalf("PostgreSQL reset failed: %v", err)
	}
}

func TestProviderBudgetConcurrentDatabaseConnections(t *testing.T) {
	ctx := context.Background()
	cfg := config.StorageConfig{Engine: "sqlite", SQLite: config.SQLiteConfig{
		Path: filepath.Join(t.TempDir(), "concurrent.db"),
	}}
	first, err := storage.Open(ctx, cfg, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err := first.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := storage.Open(ctx, cfg, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	repo := indexers.NewSQLiteRepository(first.DB())
	for _, id := range []string{"one", "two"} {
		if _, err := repo.Create(ctx, indexers.Definition{ID: id, Kind: indexers.KindNull}); err != nil {
			t.Fatal(err)
		}
	}
	stores := []*indexers.ProviderBudgetStore{
		indexers.NewProviderBudgetStore(first.DB(), nil),
		indexers.NewProviderBudgetStore(second.DB(), nil),
	}
	if err := stores[0].Configure(ctx, indexers.ProviderBudget{
		ID: "shared", Allowance: 7, WindowSeconds: 60, IndexerIDs: []string{"one", "two"},
	}); err != nil {
		t.Fatal(err)
	}
	var admitted atomic.Int64
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Go(func() {
			err := stores[i%2].Reserve(ctx, []string{"one", "two"}[i%2])
			var denied *indexers.BudgetDenied
			if err == nil {
				admitted.Add(1)
			} else if !errors.As(err, &denied) {
				t.Errorf("admission error: %v", err)
			}
		})
	}
	wg.Wait()
	if admitted.Load() != 7 {
		t.Fatalf("independent connections admitted %d, want 7", admitted.Load())
	}
}
