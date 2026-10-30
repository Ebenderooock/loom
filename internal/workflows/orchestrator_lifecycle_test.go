package workflows

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type configurableDownloadStatus struct {
	mu        sync.Mutex
	downloads map[string]ActiveDownloadInfo
	err       error
}

func (m *configurableDownloadStatus) ActiveDownloads(context.Context) (map[string]ActiveDownloadInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.downloads, m.err
}

func (m *configurableDownloadStatus) set(downloads map[string]ActiveDownloadInfo, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.downloads, m.err = downloads, err
}

type researchRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *researchRecorder) fn(_ context.Context, mediaType string, ids []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		r.calls = append(r.calls, mediaType+":"+id)
	}
}

func (r *researchRecorder) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func lifecycleOrchestrator(t *testing.T, dl *configurableDownloadStatus) (*Orchestrator, *Store, *researchRecorder) {
	t.Helper()
	store := testStore(t)
	engine := NewEngine(store, &mockMediaUpdater{}, slog.Default())
	orch := NewOrchestrator(OrchestratorOpts{
		Store:          store,
		Engine:         engine,
		Logger:         slog.Default(),
		ImportFn:       (&mockImporter{paths: []string{"/media/movie.mkv"}}).fn,
		DownloadStatus: dl,
	})
	rec := &researchRecorder{}
	orch.SetResearchFn(rec.fn)
	return orch, store, rec
}

// grabMovie runs the orchestrator, creates a downloading workflow, then stops
// the Run loop so tests can drive handlers directly.
func grabMovie(t *testing.T, orch *Orchestrator, store *Store, movieID, clientID, dlID string) *Workflow {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { orch.Run(ctx); close(done) }()
	time.Sleep(20 * time.Millisecond)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{movieID})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}
	orch.Send(CmdGrabbed{WorkflowID: wf.ID, ClientID: clientID, DownloadID: dlID, Title: "Movie.2024.1080p"})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(context.Background(), wf.ID)
		return got != nil && got.State == StateDownloading
	})
	cancel()
	<-done
	return mustGet(t, store, context.Background(), wf.ID)
}

func TestDownloadFailedFailsWorkflowAndResearches(t *testing.T) {
	dl := &configurableDownloadStatus{}
	orch, store, rec := lifecycleOrchestrator(t, dl)
	ctx := context.Background()
	wf := grabMovie(t, orch, store, "movie-1", "qb", "hash-1")

	orch.handleDownloadFailed(ctx, CmdDownloadFailed{ClientID: "qb", DownloadID: "hash-1", Reason: "no progress for 30m"})

	got := mustGet(t, store, ctx, wf.ID)
	if got.State != StateFailed {
		t.Fatalf("expected failed, got %s", got.State)
	}
	if metaString(got.Metadata, MetaFailedFrom) != StateDownloading {
		t.Fatalf("expected failed_from=downloading, metadata=%s", got.Metadata)
	}
	if active, _ := store.FindActiveForMedia(ctx, MediaTypeMovie, "movie-1"); active != nil {
		t.Fatalf("media still owned by active workflow %s (%s)", active.ID, active.State)
	}
	waitForCondition(t, 2*time.Second, func() bool { return len(rec.get()) == 1 })
	if c := rec.get(); c[0] != "movie:movie-1" {
		t.Fatalf("unexpected research call %v", c)
	}
}

func TestMarkFailedDuringDownloadIsTerminal(t *testing.T) {
	dl := &configurableDownloadStatus{}
	orch, store, _ := lifecycleOrchestrator(t, dl)
	ctx := context.Background()
	wf := grabMovie(t, orch, store, "movie-2", "qb", "hash-2")

	if err := orch.engine.markFailed(ctx, wf.ID, "boom"); err != nil {
		t.Fatalf("markFailed: %v", err)
	}
	// Previously this re-opened the workflow in "searching", which nothing
	// services, leaving the title stuck and blocking new grabs.
	if got := mustGet(t, store, ctx, wf.ID); got.State != StateFailed {
		t.Fatalf("expected failed, got %s", got.State)
	}
}

func TestAbandonedDownloadRecoversWhenItCompletes(t *testing.T) {
	dl := &configurableDownloadStatus{}
	orch, store, _ := lifecycleOrchestrator(t, dl)
	ctx := context.Background()
	wf := grabMovie(t, orch, store, "movie-3", "qb", "hash-3")

	orch.handleDownloadFailed(ctx, CmdDownloadFailed{ClientID: "qb", DownloadID: "hash-3", Reason: "stalled"})
	orch.handleDownloadComplete(ctx, CmdDownloadComplete{ClientID: "qb", DownloadID: "hash-3", Title: "Movie.2024.1080p"})

	if got := mustGet(t, store, ctx, wf.ID); got.State != StatePostDownload {
		t.Fatalf("expected post_download after late completion, got %s", got.State)
	}
}

func TestAbandonedDownloadNotRecoveredWhenReplaced(t *testing.T) {
	dl := &configurableDownloadStatus{}
	orch, store, _ := lifecycleOrchestrator(t, dl)
	ctx := context.Background()
	wf := grabMovie(t, orch, store, "movie-4", "qb", "hash-4")
	orch.handleDownloadFailed(ctx, CmdDownloadFailed{ClientID: "qb", DownloadID: "hash-4", Reason: "stalled"})

	replacement := grabMovie(t, orch, store, "movie-4", "qb", "hash-5")
	orch.handleDownloadComplete(ctx, CmdDownloadComplete{ClientID: "qb", DownloadID: "hash-4"})

	if got := mustGet(t, store, ctx, wf.ID); got.State != StateFailed {
		t.Fatalf("old workflow must stay failed when replaced, got %s", got.State)
	}
	if got := mustGet(t, store, ctx, replacement.ID); got.State != StateDownloading {
		t.Fatalf("replacement should be unaffected, got %s", got.State)
	}
}

func TestMissingDownloadAbandonedOnlyWhenClientReachable(t *testing.T) {
	dl := &configurableDownloadStatus{downloads: map[string]ActiveDownloadInfo{}}
	orch, store, rec := lifecycleOrchestrator(t, dl)
	ctx := context.Background()
	wf := grabMovie(t, orch, store, "movie-5", "qb", "hash-6")

	// Client unreachable: never abandoned, however long it's missing.
	dl.set(map[string]ActiveDownloadInfo{}, &PartialDownloadStatusError{ClientErrors: map[string]string{"qb": "timeout"}})
	orch.checkMissingDownloads(ctx)
	orch.missingSince[wf.ID] = time.Now().Add(-time.Hour)
	orch.checkMissingDownloads(ctx)
	if got := mustGet(t, store, ctx, wf.ID); got.State != StateDownloading {
		t.Fatalf("download on unreachable client abandoned: %s", got.State)
	}

	// Client reachable and download gone: abandoned after the grace period.
	dl.set(map[string]ActiveDownloadInfo{}, nil)
	orch.checkMissingDownloads(ctx)
	if got := mustGet(t, store, ctx, wf.ID); got.State != StateDownloading {
		t.Fatalf("abandoned before grace period: %s", got.State)
	}
	orch.missingSince[wf.ID] = time.Now().Add(-MissingDownloadGrace - time.Minute)
	orch.checkMissingDownloads(ctx)
	if got := mustGet(t, store, ctx, wf.ID); got.State != StateFailed {
		t.Fatalf("expected vanished download to fail, got %s", got.State)
	}
	waitForCondition(t, 2*time.Second, func() bool { return len(rec.get()) == 1 })
}

func TestMissingDownloadReappearingResetsGrace(t *testing.T) {
	dl := &configurableDownloadStatus{downloads: map[string]ActiveDownloadInfo{}}
	orch, store, _ := lifecycleOrchestrator(t, dl)
	ctx := context.Background()
	wf := grabMovie(t, orch, store, "movie-6", "qb", "hash-7")

	orch.checkMissingDownloads(ctx)
	dl.set(map[string]ActiveDownloadInfo{"qb:hash-7": {Status: "downloading"}}, nil)
	orch.checkMissingDownloads(ctx)
	if _, tracked := orch.missingSince[wf.ID]; tracked {
		t.Fatalf("missing timer not cleared after download reappeared")
	}
}

func TestReconcileIgnoresUnreachableClients(t *testing.T) {
	dl := &configurableDownloadStatus{downloads: map[string]ActiveDownloadInfo{}}
	orch, store, _ := lifecycleOrchestrator(t, dl)
	ctx := context.Background()
	wf := grabMovie(t, orch, store, "movie-7", "qb", "hash-8")

	dl.set(map[string]ActiveDownloadInfo{}, &PartialDownloadStatusError{ClientErrors: map[string]string{"qb": "connection refused"}})
	orch.reconcileOnBoot(ctx)

	if got := mustGet(t, store, ctx, wf.ID); got.State != StateDownloading {
		t.Fatalf("download on unreachable client changed at boot: %s", got.State)
	}
}

func TestStaleKeepsDownloadOnUnreachableClient(t *testing.T) {
	dl := &configurableDownloadStatus{downloads: map[string]ActiveDownloadInfo{}}
	orch, store, _ := lifecycleOrchestrator(t, dl)
	ctx := context.Background()
	wf := grabMovie(t, orch, store, "movie-8", "qb", "hash-9")

	backdate := time.Now().Add(-DownloadingStaleThreshold - time.Hour)
	if _, err := store.db.ExecContext(ctx, `UPDATE workflows SET updated_at = ? WHERE id = ?`, backdate, wf.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	dl.set(map[string]ActiveDownloadInfo{}, &PartialDownloadStatusError{ClientErrors: map[string]string{"qb": "timeout"}})
	orch.handleStale(ctx)
	if got := mustGet(t, store, ctx, wf.ID); got.State != StateDownloading {
		t.Fatalf("stale handler abandoned download on unreachable client: %s", got.State)
	}
}
