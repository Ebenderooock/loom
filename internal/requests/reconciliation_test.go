package requests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/auth"
)

type reacquiringFulfiller struct {
	fakeFulfiller
	reacquired atomic.Int32
}

func (f *reacquiringFulfiller) Reacquire(context.Context, MediaType, string, []string) error {
	f.reacquired.Add(1)
	return nil
}

type availabilityFixture struct {
	svc        *Service
	f          *reacquiringFulfiller
	r          Request
	root, path string
}

func newAvailabilityFixture(t *testing.T) availabilityFixture {
	t.Helper()
	f := &reacquiringFulfiller{}
	svc := newSvc(t, f, okValidator{})
	r := approvedRequest(t, svc.store, MediaMovie, "movie")
	execAvailabilitySQL(t, svc.store.db, `INSERT INTO movies(id,title,tmdb_id) VALUES('movie','Movie','movie')`)
	execAvailabilitySQL(t, svc.store.db, `INSERT INTO movie_files(id,movie_id,file_path) VALUES('file','movie','/movie.mkv')`)
	prepareAvailabilityFiles(context.Background(), t, svc.store.db)
	var root, path string
	if err := svc.store.db.QueryRow(`SELECT path FROM libraries WHERE id='availability-test'`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if err := svc.store.db.QueryRow(`SELECT file_path FROM movie_files WHERE id='file'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	f.movieExists = map[string]string{"movie": "movie"}
	if err := svc.ReconcileAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Get(context.Background(), r.ID)
	if err != nil || r.Status != StatusAvailable || r.Availability != AvailabilityPresent || r.FulfilledAt == nil {
		t.Fatalf("initial fulfillment: %+v %v", r, err)
	}
	return availabilityFixture{svc: svc, f: f, r: r, root: root, path: path}
}

func requestAgain(r Request) CreateInput {
	return CreateInput{MediaType: r.MediaType, TMDBID: r.TMDBID, Title: r.Title, ReRequestOf: r.ID}
}

func reconcileOne(t *testing.T, fx availabilityFixture, want Availability) Request {
	t.Helper()
	if err := fx.svc.ReconcileAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := fx.svc.Get(context.Background(), fx.r.ID)
	if err != nil || got.Availability != want {
		t.Fatalf("availability=%+v err=%v, want %s", got, err, want)
	}
	if got.Status != StatusAvailable || !got.FulfilledAt.Equal(*fx.r.FulfilledAt) || got.UserID != fx.r.UserID {
		t.Fatalf("history changed: before=%+v after=%+v", fx.r, got)
	}
	return got
}

func TestDeletionPreservesFulfillmentAndFileEvidence(t *testing.T) {
	for _, removeCatalog := range []bool{false, true} {
		t.Run(map[bool]string{false: "file deletion", true: "catalog deletion"}[removeCatalog], func(t *testing.T) {
			fx := newAvailabilityFixture(t)
			if err := os.Remove(fx.path); err != nil {
				t.Fatal(err)
			}
			if removeCatalog {
				execAvailabilitySQL(t, fx.svc.store.db, `DELETE FROM movie_files WHERE movie_id='movie'`)
				execAvailabilitySQL(t, fx.svc.store.db, `DELETE FROM movies WHERE id='movie'`)
			}
			got := reconcileOne(t, fx, AvailabilityMissing)
			if got.AvailabilityFiles == "" || got.AvailabilityLibraryID == "" {
				t.Fatalf("lost evidence: %+v", got)
			}
		})
	}
}

func TestMountLossAndRecoveryNeverConfirmsDeletion(t *testing.T) {
	fx := newAvailabilityFixture(t)
	offline := fx.root + "-offline"
	if err := os.Rename(fx.root, offline); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(fx.root); _ = os.Rename(offline, fx.root) })
	if err := os.Mkdir(fx.root, 0700); err != nil {
		t.Fatal(err)
	}
	reconcileOne(t, fx, AvailabilityUnknown)
	if _, err := fx.svc.Create(context.Background(), "1", "alice", false, requestAgain(fx.r)); !errors.Is(err, ErrConflict) {
		t.Fatalf("unmounted re-request: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fx.root, storageMarker)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disconnected mount was re-anchored: %v", err)
	}
	if err := os.Remove(fx.root); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(offline, fx.root); err != nil {
		t.Fatal(err)
	}
	reconcileOne(t, fx, AvailabilityPresent)
}

func TestReRequestAuthorizationQuotaDedupeAndAcquisition(t *testing.T) {
	fx := newAvailabilityFixture(t)
	ctx := context.Background()
	if err := os.Remove(fx.path); err != nil {
		t.Fatal(err)
	}
	reconcileOne(t, fx, AvailabilityMissing)
	if _, err := fx.svc.Create(ctx, "2", "bob", false, requestAgain(fx.r)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-owner: %v", err)
	}
	_, err := fx.svc.SetQuotaConfig(ctx, QuotaConfig{MovieLimit: 1, WindowDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Create(ctx, "1", "alice", false, requestAgain(fx.r)); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("historical fulfillment did not count against quota: %v", err)
	}
	_, err = fx.svc.SetQuotaConfig(ctx, QuotaConfig{MovieLimit: 2, WindowDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	newRequest, err := fx.svc.Create(ctx, "1", "alice", false, requestAgain(fx.r))
	if err != nil || newRequest.ReRequestOf != fx.r.ID || newRequest.ID == fx.r.ID {
		t.Fatalf("re-request: %+v %v", newRequest, err)
	}
	if _, err := fx.svc.Create(ctx, "2", "admin", true, requestAgain(fx.r)); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("global active duplicate, including admin: %v", err)
	}
	out, err := fx.svc.Approve(ctx, newRequest.ID, "qp", "lib", "admin")
	if err != nil || out.Status != StatusApproved || fx.f.reacquired.Load() != 1 {
		t.Fatalf("re-acquisition: %+v err=%v calls=%d", out, err, fx.f.reacquired.Load())
	}
	quota, err := fx.svc.QuotaStatus(ctx, "1", false)
	if err != nil || quota.Movie.Used != 2 {
		t.Fatalf("quota=%+v err=%v", quota, err)
	}
	reconcileOne(t, fx, AvailabilityMissing)
	if err := os.WriteFile(fx.path, []byte("new media"), 0600); err != nil {
		t.Fatal(err)
	}
	reconcileOne(t, fx, AvailabilityPresent)
	out, err = fx.svc.Get(ctx, newRequest.ID)
	if err != nil || out.Status != StatusAvailable || out.FulfilledAt == nil {
		t.Fatalf("replacement not fulfilled: %+v %v", out, err)
	}
}

func TestConcurrentReRequestsCreateOneGlobalActiveRequest(t *testing.T) {
	fx := newAvailabilityFixture(t)
	if err := os.Remove(fx.path); err != nil {
		t.Fatal(err)
	}
	var won atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := fx.svc.Create(context.Background(), "1", "alice", false, requestAgain(fx.r))
			if err == nil {
				won.Add(1)
			} else if !errors.Is(err, ErrDuplicate) {
				t.Errorf("create: %v", err)
			}
		})
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("created %d requests", won.Load())
	}
}

func TestApprovalRechecksStorageAndAvoidsAcquiringRecoveredFiles(t *testing.T) {
	for _, recover := range []bool{false, true} {
		t.Run(map[bool]string{false: "mount disappears", true: "file recovers"}[recover], func(t *testing.T) {
			fx := newAvailabilityFixture(t)
			ctx := context.Background()
			if err := os.Remove(fx.path); err != nil {
				t.Fatal(err)
			}
			r, err := fx.svc.Create(ctx, "99", "admin", true, requestAgain(fx.r))
			if err != nil {
				t.Fatal(err)
			}
			if recover {
				if err := os.WriteFile(fx.path, []byte("media"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Remove(filepath.Join(fx.root, storageMarker)); err != nil {
					t.Fatal(err)
				}
			}
			out, err := fx.svc.Approve(ctx, r.ID, "qp", "lib", "admin")
			if recover && (err != nil || out.Status != StatusAvailable) {
				t.Fatalf("recovery: %+v %v", out, err)
			}
			if !recover && !errors.Is(err, ErrConflict) {
				t.Fatalf("mount-loss approval: %v", err)
			}
			if fx.f.reacquired.Load() != 0 {
				t.Fatal("acquired without confirmed missing files")
			}
		})
	}
}

func TestReconciliationDryRunAuditIdempotenceAndStaleObservations(t *testing.T) {
	fx := newAvailabilityFixture(t)
	ctx := context.Background()
	if err := os.Remove(fx.path); err != nil {
		t.Fatal(err)
	}
	run, err := fx.svc.store.Reconcile(ctx, time.Now().UTC(), "admin", true)
	if err != nil || !run.DryRun || len(run.Outcomes) != 1 || run.Outcomes[0].After != AvailabilityMissing || !run.Outcomes[0].Changed {
		t.Fatalf("preview=%+v err=%v", run, err)
	}
	got, err := fx.svc.Get(ctx, fx.r.ID)
	if err != nil || got.Availability != AvailabilityPresent {
		t.Fatalf("dry-run changed request: %+v %v", got, err)
	}
	reconcileOne(t, fx, AvailabilityMissing)
	before, err := fx.svc.Get(ctx, fx.r.ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err = fx.svc.store.Reconcile(ctx, time.Now().UTC().Add(time.Second), "admin", false)
	if err != nil || run.Outcomes[0].Changed {
		t.Fatalf("not idempotent: %+v %v", run, err)
	}
	got, err = fx.svc.Get(ctx, fx.r.ID)
	if err != nil || !got.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("repeat changed history: %+v %v", got, err)
	}
	history, err := fx.svc.store.ReconciliationHistory(ctx)
	if err != nil || len(history) < 3 {
		t.Fatalf("history=%+v %v", history, err)
	}
	var previewFound bool
	for _, r := range history {
		previewFound = previewFound || r.ID == run.ID || r.DryRun
	}
	if !previewFound {
		t.Fatal("missing audit")
	}
	execAvailabilitySQL(t, fx.svc.store.db, `UPDATE media_requests SET availability_checked_at=? WHERE id=?`,
		time.Now().UTC().Add(-10*time.Minute).Format(tsLayout), fx.r.ID)
	got, err = fx.svc.Get(ctx, fx.r.ID)
	if err != nil || got.Availability != AvailabilityUnknown {
		t.Fatalf("stale scan reported absence: %+v %v", got, err)
	}
}

func TestReconcileEndpointsRequireAdminAndExplicitDryRun(t *testing.T) {
	fx := newAvailabilityFixture(t)
	adminOnly := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := auth.IdentityFrom(r.Context())
			if id == nil || !id.HasRole("admin") {
				w.WriteHeader(http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
	router := Router(fx.svc, adminOnly)
	for _, tc := range []struct {
		role, body string
		want       int
	}{
		{"user", `{"dry_run":true}`, 403},
		{"admin", `{}`, 400},
		{"admin", `{"dry_run":true}`, 200},
	} {
		req := httptest.NewRequest(http.MethodPost, "/reconcile", strings.NewReader(tc.body))
		req = req.WithContext(auth.WithIdentity(req.Context(), &auth.Identity{UserID: 1, Username: "alice", Roles: []string{tc.role}}))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Fatalf("%s %s: %d %s", tc.role, tc.body, rr.Code, rr.Body.String())
		}
	}
}

func TestPartialSeriesDeletionAndRecoveryRetainFulfillment(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	r := approvedRequest(t, store, MediaSeries, "series")
	execAvailabilitySQL(t, db, `INSERT INTO series(id,title) VALUES('series','Series')`)
	execAvailabilitySQL(t, db, `INSERT INTO seasons(id,series_id,season_number) VALUES('season','series',1)`)
	execAvailabilitySQL(t, db, `INSERT INTO episodes(id,series_id,season_id,episode_number,air_date)
		VALUES('one','series','season',1,'2020-01-01'),('two','series','season',2,'2020-01-02'),
		('future','series','season',3,'2099-01-01')`)
	execAvailabilitySQL(t, db, `INSERT INTO episode_files(id,episode_id,series_id,file_path)
		VALUES('one-file','one','series','/one.mkv')`)
	prepareAvailabilityFiles(context.Background(), t, db)
	ctx := context.Background()
	if err := store.ReconcileAvailability(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, r.ID)
	if err != nil || got.Status != StatusApproved || got.Availability == AvailabilityPresent {
		t.Fatalf("partial series=%+v %v", got, err)
	}
	execAvailabilitySQL(t, db, `INSERT INTO episode_files(id,episode_id,series_id,file_path) VALUES('two-file','two','series','/two.mkv')`)
	prepareAvailabilityFiles(context.Background(), t, db)
	if err := store.ReconcileAvailability(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(ctx, r.ID)
	if err != nil || got.Status != StatusAvailable || got.Availability != AvailabilityPresent || got.FulfilledAt == nil {
		t.Fatalf("complete series=%+v %v", got, err)
	}
	fulfilled := *got.FulfilledAt
	var path string
	if err := db.QueryRow(`SELECT file_path FROM episode_files WHERE id='two-file'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	execAvailabilitySQL(t, db, `DELETE FROM episode_files WHERE id='two-file'`)
	if err := store.ReconcileAvailability(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(ctx, r.ID)
	if err != nil || got.Status != StatusAvailable || got.Availability != AvailabilityMissing || !got.FulfilledAt.Equal(fulfilled) {
		t.Fatalf("deleted episode=%+v %v", got, err)
	}
	execAvailabilitySQL(t, db, `INSERT INTO episode_files(id,episode_id,series_id,file_path) VALUES('two-file','two','series',?)`, path)
	if err := os.WriteFile(path, []byte("media"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileAvailability(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(ctx, r.ID)
	if err != nil || got.Availability != AvailabilityPresent || !got.FulfilledAt.Equal(fulfilled) {
		t.Fatalf("recovered episode=%+v %v", got, err)
	}
}

func TestDryRunBootstrapAndUntrustedRootNeverWriteMarker(t *testing.T) {
	fx := newAvailabilityFixture(t)
	ctx := context.Background()
	execAvailabilitySQL(t, fx.svc.store.db, `DELETE FROM request_storage_anchors`)
	if err := os.Remove(filepath.Join(fx.root, storageMarker)); err != nil {
		t.Fatal(err)
	}
	run, err := fx.svc.store.Reconcile(ctx, time.Now().UTC(), "admin", true)
	if err != nil || run.Outcomes[0].After != AvailabilityPresent {
		t.Fatalf("bootstrap preview=%+v %v", run, err)
	}
	if _, err := os.Stat(filepath.Join(fx.root, storageMarker)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry run wrote marker: %v", err)
	}
	var n int
	if err := fx.svc.store.db.QueryRow(`SELECT COUNT(*) FROM request_storage_anchors`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("dry run wrote anchor: %d %v", n, err)
	}
	if err := os.Remove(fx.path); err != nil {
		t.Fatal(err)
	}
	reconcileOne(t, fx, AvailabilityUnknown)
}
