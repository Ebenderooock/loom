package requests

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(dir, "loom.db")},
	}
	db, err := storage.Open(context.Background(), cfg,
		slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db.DB()
}

// fakeFulfiller is a controllable Fulfiller for service tests.
type fakeFulfiller struct {
	mu             sync.Mutex
	movieExists    map[string]string
	seriesExists   map[string]string
	fulfillMovieN  int32
	fulfillErr     error
	fulfilledMedia string
}

func (f *fakeFulfiller) MovieExists(_ context.Context, tmdbID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.movieExists[tmdbID], nil
}

func (f *fakeFulfiller) SeriesExists(_ context.Context, tmdbID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seriesExists[tmdbID], nil
}

func (f *fakeFulfiller) FulfillMovie(_ context.Context, _, _, _ string) (string, error) {
	atomic.AddInt32(&f.fulfillMovieN, 1)
	if f.fulfillErr != nil {
		return "", f.fulfillErr
	}
	return f.fulfilledMedia, nil
}

func (f *fakeFulfiller) FulfillSeries(_ context.Context, _, _, _ string) (string, error) {
	if f.fulfillErr != nil {
		return "", f.fulfillErr
	}
	return f.fulfilledMedia, nil
}

// okValidator accepts every target.
type okValidator struct{}

func (okValidator) ValidateTarget(context.Context, MediaType, string, string) error { return nil }

func newSvc(t *testing.T, f Fulfiller, v LibraryValidator) *Service {
	t.Helper()
	return NewService(Options{
		Store:     NewStore(openTestDB(t)),
		Fulfiller: f,
		Validator: v,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestCreateAndListMine(t *testing.T) {
	svc := newSvc(t, &fakeFulfiller{}, okValidator{})
	ctx := context.Background()

	r, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603", Title: "The Matrix"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.Status != StatusPending {
		t.Fatalf("status = %q, want pending", r.Status)
	}

	mine, err := svc.ListMine(ctx, "1")
	if err != nil {
		t.Fatalf("ListMine: %v", err)
	}
	if len(mine) != 1 || mine[0].Title != "The Matrix" {
		t.Fatalf("ListMine = %+v", mine)
	}
}

func TestCreateRejectsInvalid(t *testing.T) {
	svc := newSvc(t, &fakeFulfiller{}, okValidator{})
	ctx := context.Background()

	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: "music", TMDBID: "1"}); err == nil {
		t.Fatal("expected invalid media type error")
	}
	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "  "}); err == nil {
		t.Fatal("expected missing tmdb error")
	}
}

func TestCreateDuplicateOpen(t *testing.T) {
	svc := newSvc(t, &fakeFulfiller{}, okValidator{})
	ctx := context.Background()

	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := svc.Create(ctx, "2", "bob", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"})
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("err = %v, want ErrDuplicate", err)
	}
}

func TestCreateAlreadyAvailable(t *testing.T) {
	f := &fakeFulfiller{movieExists: map[string]string{"603": "movie-1"}}
	svc := newSvc(t, f, okValidator{})
	_, err := svc.Create(context.Background(), "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"})
	if !errors.Is(err, ErrAlreadyAvailable) {
		t.Fatalf("err = %v, want ErrAlreadyAvailable", err)
	}
}

func TestApproveHappyPath(t *testing.T) {
	f := &fakeFulfiller{fulfilledMedia: "movie-99"}
	svc := newSvc(t, f, okValidator{})
	ctx := context.Background()

	r, _ := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"})
	out, err := svc.Approve(ctx, r.ID, "qp-1", "lib-1", "admin")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if out.Status != StatusApproved || out.MediaID != "movie-99" {
		t.Fatalf("out = %+v", out)
	}
	if got := atomic.LoadInt32(&f.fulfillMovieN); got != 1 {
		t.Fatalf("FulfillMovie called %d times, want 1", got)
	}
}

func TestApproveValidatesTarget(t *testing.T) {
	svc := newSvc(t, &fakeFulfiller{}, rejectValidator{})
	ctx := context.Background()
	r, _ := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"})
	if _, err := svc.Approve(ctx, r.ID, "bad-qp", "bad-lib", "admin"); err == nil {
		t.Fatal("expected validation error")
	}
	// Request must remain pending (not claimed) after a validation failure.
	got, _ := svc.Store().Get(ctx, r.ID)
	if got.Status != StatusPending {
		t.Fatalf("status = %q, want pending", got.Status)
	}
}

type rejectValidator struct{}

func (rejectValidator) ValidateTarget(context.Context, MediaType, string, string) error {
	return errors.New("nope")
}

func TestApproveFulfillFailureMarksFailed(t *testing.T) {
	f := &fakeFulfiller{fulfillErr: errors.New("boom")}
	svc := newSvc(t, f, okValidator{})
	ctx := context.Background()
	r, _ := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"})
	if _, err := svc.Approve(ctx, r.ID, "qp", "lib", "admin"); err == nil {
		t.Fatal("expected fulfillment error")
	}
	got, _ := svc.Store().Get(ctx, r.ID)
	if got.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	// A failed request is re-requestable.
	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"}); err != nil {
		t.Fatalf("resubmit after failure: %v", err)
	}
}

func TestApproveAlreadyExistingShortCircuits(t *testing.T) {
	f := &fakeFulfiller{}
	svc := newSvc(t, f, okValidator{})
	ctx := context.Background()
	r, _ := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"})
	// Media appears in the library between create and approve.
	f.mu.Lock()
	f.movieExists = map[string]string{"603": "movie-7"}
	f.mu.Unlock()

	out, err := svc.Approve(ctx, r.ID, "qp", "lib", "admin")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if out.Status != StatusAvailable || out.MediaID != "movie-7" {
		t.Fatalf("out = %+v", out)
	}
	if got := atomic.LoadInt32(&f.fulfillMovieN); got != 0 {
		t.Fatalf("FulfillMovie called %d times, want 0", got)
	}
}

func TestApproveConcurrentClaimsOnce(t *testing.T) {
	f := &fakeFulfiller{fulfilledMedia: "movie-1"}
	svc := newSvc(t, f, okValidator{})
	ctx := context.Background()
	r, _ := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"})

	var wg sync.WaitGroup
	var success int32
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Approve(ctx, r.ID, "qp", "lib", "admin"); err == nil {
				atomic.AddInt32(&success, 1)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&f.fulfillMovieN); got != 1 {
		t.Fatalf("FulfillMovie called %d times, want exactly 1", got)
	}
	if got := atomic.LoadInt32(&success); got != 1 {
		t.Fatalf("%d approvals succeeded, want exactly 1", got)
	}
}

func TestRejectAfterApproveFails(t *testing.T) {
	f := &fakeFulfiller{fulfilledMedia: "movie-1"}
	svc := newSvc(t, f, okValidator{})
	ctx := context.Background()
	r, _ := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"})
	if _, err := svc.Approve(ctx, r.ID, "qp", "lib", "admin"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := svc.Reject(ctx, r.ID, "too late", "admin"); err == nil {
		t.Fatal("expected reject of approved request to fail")
	}
}

func TestReject(t *testing.T) {
	svc := newSvc(t, &fakeFulfiller{}, okValidator{})
	ctx := context.Background()
	r, _ := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"})
	out, err := svc.Reject(ctx, r.ID, "not now", "admin")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if out.Status != StatusRejected || out.Reason != "not now" {
		t.Fatalf("out = %+v", out)
	}
	// Rejected request is re-requestable.
	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"}); err != nil {
		t.Fatalf("resubmit after reject: %v", err)
	}
}

func TestQuotaEnforcement(t *testing.T) {
	svc := newSvc(t, &fakeFulfiller{}, okValidator{})
	ctx := context.Background()

	if _, err := svc.SetQuotaConfig(ctx, QuotaConfig{MovieLimit: 2, SeriesLimit: 0, WindowDays: 7}); err != nil {
		t.Fatalf("SetQuotaConfig: %v", err)
	}

	// Two distinct movie requests are allowed.
	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "1"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "2"}); err != nil {
		t.Fatalf("second: %v", err)
	}
	// Third exceeds the movie quota.
	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "3"}); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("third movie: expected ErrQuotaExceeded, got %v", err)
	}
	// Series is unlimited (limit 0) so it still works.
	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaSeries, TMDBID: "10"}); err != nil {
		t.Fatalf("series unlimited: %v", err)
	}
	// A different user has their own quota.
	if _, err := svc.Create(ctx, "2", "bob", false, CreateInput{MediaType: MediaMovie, TMDBID: "4"}); err != nil {
		t.Fatalf("other user: %v", err)
	}
}

func TestQuotaAdminBypass(t *testing.T) {
	svc := newSvc(t, &fakeFulfiller{}, okValidator{})
	ctx := context.Background()
	if _, err := svc.SetQuotaConfig(ctx, QuotaConfig{MovieLimit: 1, WindowDays: 7}); err != nil {
		t.Fatalf("SetQuotaConfig: %v", err)
	}
	for i, tmdb := range []string{"1", "2", "3"} {
		if _, err := svc.Create(ctx, "9", "admin", true, CreateInput{MediaType: MediaMovie, TMDBID: tmdb}); err != nil {
			t.Fatalf("admin create %d: %v", i, err)
		}
	}
}

func TestQuotaRejectedDoesNotCount(t *testing.T) {
	svc := newSvc(t, &fakeFulfiller{}, okValidator{})
	ctx := context.Background()
	if _, err := svc.SetQuotaConfig(ctx, QuotaConfig{MovieLimit: 1, WindowDays: 7}); err != nil {
		t.Fatalf("SetQuotaConfig: %v", err)
	}
	r, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "1"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// At the limit now.
	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "2"}); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}
	// Rejecting the first frees the slot.
	if _, err := svc.Reject(ctx, r.ID, "no", "admin"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "2"}); err != nil {
		t.Fatalf("after reject: %v", err)
	}
}

func TestQuotaStatusReporting(t *testing.T) {
	svc := newSvc(t, &fakeFulfiller{}, okValidator{})
	ctx := context.Background()
	if _, err := svc.SetQuotaConfig(ctx, QuotaConfig{MovieLimit: 5, SeriesLimit: 0, WindowDays: 7}); err != nil {
		t.Fatalf("SetQuotaConfig: %v", err)
	}
	if _, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "1"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	st, err := svc.QuotaStatus(ctx, "1", false)
	if err != nil {
		t.Fatalf("QuotaStatus: %v", err)
	}
	if st.WindowDays != 7 {
		t.Fatalf("window = %d, want 7", st.WindowDays)
	}
	if st.Movie.Limit != 5 || st.Movie.Used != 1 || st.Movie.Remaining != 4 || st.Movie.Unlimited {
		t.Fatalf("movie quota = %+v", st.Movie)
	}
	if !st.Series.Unlimited || st.Series.Remaining != -1 {
		t.Fatalf("series quota = %+v", st.Series)
	}
	// Admin sees unlimited even with a configured limit.
	adminSt, err := svc.QuotaStatus(ctx, "9", true)
	if err != nil {
		t.Fatalf("admin QuotaStatus: %v", err)
	}
	if !adminSt.Movie.Unlimited {
		t.Fatalf("admin movie should be unlimited: %+v", adminSt.Movie)
	}
}

func TestSetQuotaConfigValidation(t *testing.T) {
	svc := newSvc(t, &fakeFulfiller{}, okValidator{})
	ctx := context.Background()
	if _, err := svc.SetQuotaConfig(ctx, QuotaConfig{MovieLimit: -1}); !errors.Is(err, ErrInvalidQuota) {
		t.Fatalf("negative limit: got %v", err)
	}
	if _, err := svc.SetQuotaConfig(ctx, QuotaConfig{MovieLimit: 3, WindowDays: 99999}); !errors.Is(err, ErrInvalidQuota) {
		t.Fatalf("huge window: got %v", err)
	}
	// Zero window defaults to DefaultWindowDays.
	saved, err := svc.SetQuotaConfig(ctx, QuotaConfig{MovieLimit: 3, WindowDays: 0})
	if err != nil {
		t.Fatalf("default window: %v", err)
	}
	if saved.WindowDays != DefaultWindowDays {
		t.Fatalf("window = %d, want %d", saved.WindowDays, DefaultWindowDays)
	}
}
