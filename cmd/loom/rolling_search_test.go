package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ebenderooock/loom/internal/autosearch"
	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/scheduler"
	"github.com/ebenderooock/loom/internal/storage"
)

type rollingTestMovies struct{ movies.Service }

func (rollingTestMovies) GetMovie(context.Context, string) (*movies.Movie, error) {
	id := "tt-test"
	return &movies.Movie{ID: "movie", Title: "Quota Test", IMDBID: &id}, nil
}
func (rollingTestMovies) ListQualityDefinitions(context.Context) ([]*movies.QualityDefinition, error) {
	return []*movies.QualityDefinition{{ID: "webdl-1080", Name: "webdl-1080p", Source: "Web", Resolution: "1080p"}}, nil
}
func (rollingTestMovies) ListMovieFiles(context.Context, string) ([]*movies.MovieFile, error) {
	return nil, nil
}

type rollingTestIndexer struct {
	calls   atomic.Int64
	fail    bool
	cancel  context.CancelFunc
	results []indexers.Result
}

func (*rollingTestIndexer) ID() string   { return "provider" }
func (*rollingTestIndexer) Name() string { return "Provider" }
func (*rollingTestIndexer) Caps() indexers.Caps {
	return indexers.Caps{SupportedIDs: []string{"imdb"}}
}
func (*rollingTestIndexer) Test(context.Context) error { return nil }
func (p *rollingTestIndexer) Search(ctx context.Context, _ indexers.Query) (*indexers.Results, error) {
	p.calls.Add(1)
	if p.cancel != nil {
		p.cancel()
		return nil, ctx.Err()
	}
	if p.fail {
		return nil, errors.New("provider unavailable")
	}
	return &indexers.Results{Items: p.results}, nil
}

func rollingFixture(t *testing.T, max int, p *rollingTestIndexer) (*scheduler.RollingSearcher, *autosearch.Engine, *indexers.Service) {
	t.Helper()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(ctx, config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "loom.db")},
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	profiles := qualityprofiles.NewStore(db.DB())
	if err := profiles.Create(ctx, &qualityprofiles.QualityProfile{
		ID: "profile", Name: "Test", Items: `[{"id":"webdl-1080","allowed":true}]`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO movies (id, title, status, quality_profile_id, imdb_id)
		VALUES ('movie', 'Quota Test', 'missing', 'profile', 'tt-test')`); err != nil {
		t.Fatal(err)
	}
	reg := indexers.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	svc, err := indexers.NewService(indexers.ServiceOptions{
		Repository: indexers.NewSQLiteRepository(db.DB()), Registry: reg, Logger: logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := autosearch.NewEngine(svc, profiles, customformats.NewEngine(nil), nil,
		downloads.NewRegistry(), rollingTestMovies{}, nil, logger)
	cfg := scheduler.DefaultRollingSearchConfig()
	cfg.MaxSearchesPerDay = max
	rs := scheduler.NewRollingSearcher(scheduler.NewStore(db.DB()), svc, logger, cfg)
	rs.SetGrabber(&autoSearchGrabber{engine: engine})
	return rs, engine, svc
}

func TestRollingSearchProductionGrabberQuotaAndRetry(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		max                                int
		fail, cancelled                    bool
		wantCalls, wantSearched, wantQueue int
	}{
		{"empty success gets cooldown", 10, false, false, 2, 1, 0},
		{"ID exhausts quota before title", 1, false, false, 1, 1, 0},
		{"provider failures remain retryable", 1, true, false, 1, 0, 1},
		{"cancelled search remains retryable", 10, false, true, 1, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := &rollingTestIndexer{fail: tc.fail}
			if tc.cancelled {
				p.cancel = cancel
			}
			rs, _, _ := rollingFixture(t, tc.max, p)
			rs.Trigger(ctx)
			status := rs.Status(context.Background())
			if got := p.calls.Load(); got != int64(tc.wantCalls) {
				t.Fatalf("provider calls = %d, want %d", got, tc.wantCalls)
			}
			if status.ItemsSearched != tc.wantSearched || status.ItemsInQueue != tc.wantQueue {
				t.Fatalf("unexpected status: %+v", status)
			}
			if status.QuotaUsage["provider"] != tc.wantCalls {
				t.Fatalf("attempts, including errors, must consume quota: %+v", status)
			}
			if !tc.cancelled {
				rs.Trigger(context.Background())
				if p.calls.Load() != int64(tc.wantCalls) {
					t.Fatal("cooldown or exhausted quota allowed another query")
				}
			}
		})
	}
}

func TestRollingAdmissionCoversAlternateQueriesAndLeavesManualSearchUnchanged(t *testing.T) {
	p := &rollingTestIndexer{}
	_, engine, _ := rollingFixture(t, 10, p)
	quota := scheduler.NewQuotaTracker(2)
	ctx := indexers.WithSearchAdmission(context.Background(), quota.TryReserve)
	req := autosearch.SearchRequest{
		Title: "Quota Test", MediaType: "movie", IMDBID: "tt-test",
		AlternateTitles:  []string{"Alternate One", "Alternate Two"},
		QualityProfileID: "profile", RequireSuccessfulSearch: true,
	}
	if _, err := engine.SearchAndGrab(ctx, req); err != nil {
		t.Fatal(err)
	}
	if p.calls.Load() != 2 || quota.Usage()["provider"] != 2 {
		t.Fatalf("ID/title/alternate fanout exceeded quota: %d", p.calls.Load())
	}
	if _, err := engine.SearchAndGrab(ctx, req); err == nil {
		t.Fatal("all-quota-skipped background search must be retryable")
	}
	if _, err := engine.SearchAndGrab(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if p.calls.Load() != 6 {
		t.Fatalf("manual context should not inherit quota: %d calls", p.calls.Load())
	}
}

func TestRollingFailurePolicyDoesNotChangeManualNoMatch(t *testing.T) {
	p := &rollingTestIndexer{fail: true}
	_, engine, svc := rollingFixture(t, 10, p)
	req := autosearch.SearchRequest{
		Title: "Quota Test", MediaType: "movie", QualityProfileID: "profile",
	}

	if _, err := engine.SearchAndGrab(context.Background(), req); err != nil {
		t.Fatalf("manual partial-result behavior changed: %v", err)
	}
	// Reset availability so this verifies provider errors, not circuit skipping.
	svc.Availability().RecordSuccess(p.ID())
	req.RequireSuccessfulSearch = true
	if _, err := engine.SearchAndGrab(context.Background(), req); err == nil {
		t.Fatal("background all-provider failure was reported as no-match")
	}
}

func TestRollingFailedCandidateCanSucceedOnRetry(t *testing.T) {
	p := &rollingTestIndexer{fail: true}
	rs, _, svc := rollingFixture(t, 10, p)
	rs.Trigger(context.Background())
	if status := rs.Status(context.Background()); status.ItemsSearched != 0 || status.ItemsInQueue != 1 {
		t.Fatalf("failed candidate received cooldown: %+v", status)
	}
	p.fail = false
	svc.Availability().RecordSuccess(p.ID())
	rs.Trigger(context.Background())
	if status := rs.Status(context.Background()); status.ItemsSearched != 1 || status.ItemsInQueue != 0 {
		t.Fatalf("failed candidate could not be retried: %+v", status)
	}
}

func TestRollingBareSearchAlsoRequiresSuccessAndHonorsQuota(t *testing.T) {
	for _, fail := range []bool{false, true} {
		p := &rollingTestIndexer{fail: fail}
		rs, _, _ := rollingFixture(t, 1, p)
		rs.SetGrabber(nil)
		rs.Trigger(context.Background())
		rs.Trigger(context.Background())
		status := rs.Status(context.Background())
		wantSearched := 1
		if fail {
			wantSearched = 0
		}

		if p.calls.Load() != 1 || status.QuotaUsage[p.ID()] != 1 || status.ItemsSearched != wantSearched {
			t.Fatalf("bare search failed quota/retry semantics: fail=%v calls=%d status=%+v", fail, p.calls.Load(), status)
		}
	}
}

func TestRollingGrabFailureRemainsRetryable(t *testing.T) {
	p := &rollingTestIndexer{results: []indexers.Result{{
		Title: "Quota.Test.1080p.WEB-DL", MagnetURI: "magnet:?xt=urn:btih:test",
	}}}
	rs, engine, _ := rollingFixture(t, 10, p)
	// The release passes evaluation, but the empty download registry makes grab
	// fail. Background callers must receive an error, not a successful no-match.
	result, err := engine.SearchAndGrab(context.Background(), autosearch.SearchRequest{
		Title: "Quota Test", MediaType: "movie", QualityProfileID: "profile",
		RequireSuccessfulSearch: true,
	})
	if err == nil || result == nil || result.Rejected != 0 || result.Considered == 0 {
		t.Fatalf("expected retryable grab failure, got result=%+v err=%v", result, err)
	}
	rs.Trigger(context.Background())
	status := rs.Status(context.Background())
	if status.ItemsSearched != 0 || status.ItemsInQueue != 1 {
		t.Fatalf("grab failure received cooldown: %+v", status)
	}
}

func TestRollingCircuitSkippedProvidersDoNotConsumeQuotaOrCooldown(t *testing.T) {
	p := &rollingTestIndexer{}
	rs, _, svc := rollingFixture(t, 10, p)
	svc.Availability().RecordFailure(p.ID())
	svc.Availability().RecordFailure(p.ID())
	rs.Trigger(context.Background())
	status := rs.Status(context.Background())
	if p.calls.Load() != 0 || status.QuotaUsage[p.ID()] != 0 ||
		status.ItemsSearched != 0 || status.ItemsInQueue != 1 {
		t.Fatalf("circuit-skipped provider queried or cooled down: calls=%d status=%+v", p.calls.Load(), status)
	}
}
