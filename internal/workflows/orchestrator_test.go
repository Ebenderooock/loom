package workflows

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// ── Mocks ─────────────────────────────────────────────────────────────

type mockMediaUpdater struct{}

func (m *mockMediaUpdater) SetMovieDownloading(ctx context.Context, movieID string) error {
	return nil
}
func (m *mockMediaUpdater) SetMovieMissing(ctx context.Context, movieID string) error {
	return nil
}
func (m *mockMediaUpdater) SetEpisodeDownloading(ctx context.Context, episodeID string) error {
	return nil
}
func (m *mockMediaUpdater) SetEpisodeMissing(ctx context.Context, episodeID string) error {
	return nil
}

type importCall struct {
	ClientID, DownloadID, Title, Category string
}

type mockImporter struct {
	mu    sync.Mutex
	calls []importCall
	paths []string
	err   error
}

func (m *mockImporter) fn(ctx context.Context, clientID, downloadID, title, category string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, importCall{clientID, downloadID, title, category})
	return m.paths, m.err
}

func (m *mockImporter) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

type mockDownloadStatus struct {
	downloads map[string]ActiveDownloadInfo
}

func (m *mockDownloadStatus) ActiveDownloads(ctx context.Context) (map[string]ActiveDownloadInfo, error) {
	return m.downloads, nil
}

// ── Helpers ───────────────────────────────────────────────────────────

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// Single connection avoids separate in-memory databases per connection.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func testOrchestrator(t *testing.T, imp *mockImporter) (*Orchestrator, *Store) {
	t.Helper()
	store := testStore(t)
	engine := NewEngine(store, &mockMediaUpdater{}, slog.Default())
	orch := NewOrchestrator(OrchestratorOpts{
		Store:          store,
		Engine:         engine,
		Logger:         slog.Default(),
		ImportFn:       imp.fn,
		DownloadStatus: &mockDownloadStatus{downloads: map[string]ActiveDownloadInfo{}},
	})
	return orch, store
}

func startOrchestrator(t *testing.T, orch *Orchestrator) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go orch.Run(ctx)
	t.Cleanup(cancel)
	// Allow orchestrator goroutine to start.
	time.Sleep(20 * time.Millisecond)
	return ctx, cancel
}

func waitForCondition(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func mustGet(t *testing.T, store *Store, ctx context.Context, id string) *Workflow {
	t.Helper()
	wf, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("store.Get(%s): %v", id, err)
	}
	return wf
}

// backdateSettling sets the post_download start time to the past so the
// settling delay is immediately satisfied in tests.
func backdateSettling(t *testing.T, store *Store, ctx context.Context, wfID string) {
	t.Helper()
	wf := mustGet(t, store, ctx, wfID)
	policy := GetPostDownloadPolicy(wf.Metadata)
	if policy == nil {
		policy = &PostDownloadPolicy{}
	}
	policy.StartedAt = time.Now().Add(-1 * time.Minute)
	if err := store.SetPostDownloadPolicy(ctx, wfID, *policy); err != nil {
		t.Fatalf("backdate settling: %v", err)
	}
}

// ── Tests ─────────────────────────────────────────────────────────────

func TestOrchestratorStartSearch(t *testing.T) {
	imp := &mockImporter{paths: []string{"/media/movie.mkv"}}
	orch, store := testOrchestrator(t, imp)
	ctx, _ := startOrchestrator(t, orch)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{"movie-123"})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}
	if wf == nil {
		t.Fatal("expected workflow, got nil")
	}
	if wf.State != StateSearching {
		t.Fatalf("expected state %s, got %s", StateSearching, wf.State)
	}
	if wf.Type != TypeMovieSearch {
		t.Fatalf("expected type %s, got %s", TypeMovieSearch, wf.Type)
	}

	// Verify persisted in store
	got, err := store.Get(ctx, wf.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if got.State != StateSearching {
		t.Fatalf("stored state: expected %s, got %s", StateSearching, got.State)
	}
	if len(got.Items) != 1 || got.Items[0].MediaID != "movie-123" {
		t.Fatalf("items mismatch: %+v", got.Items)
	}
}

func TestOrchestratorGrabbed(t *testing.T) {
	imp := &mockImporter{paths: []string{"/media/movie.mkv"}}
	orch, store := testOrchestrator(t, imp)
	ctx, _ := startOrchestrator(t, orch)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{"movie-456"})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}

	orch.Send(CmdGrabbed{
		WorkflowID: wf.ID,
		ClientID:   "qbit-1",
		DownloadID: "dl-001",
		Title:      "Movie.2024.1080p",
	})

	// Wait for state transition to downloading (grabbed → downloading happens immediately)
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StateDownloading
	})

	got, _ := store.Get(ctx, wf.ID)
	if got.DownloadClientID != "qbit-1" {
		t.Fatalf("expected client qbit-1, got %s", got.DownloadClientID)
	}
	if got.DownloadID != "dl-001" {
		t.Fatalf("expected download dl-001, got %s", got.DownloadID)
	}
	if got.GrabTitle != "Movie.2024.1080p" {
		t.Fatalf("expected grab title Movie.2024.1080p, got %s", got.GrabTitle)
	}
}

func TestOrchestratorGrabbedUpdatesExistingDownloadingWorkflow(t *testing.T) {
	imp := &mockImporter{paths: []string{"/media/movie.mkv"}}
	orch, store := testOrchestrator(t, imp)
	ctx, _ := startOrchestrator(t, orch)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{"movie-456"})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}

	orch.Send(CmdGrabbed{
		WorkflowID: wf.ID,
		ClientID:   "qbit-1",
		DownloadID: "dl-001",
		Title:      "Movie.2024.1080p",
	})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StateDownloading
	})

	// Re-grab same media with a new download ID should update the existing
	// active workflow instead of failing on state mismatch.
	orch.Send(CmdGrabbed{
		WorkflowID: wf.ID,
		ClientID:   "qbit-1",
		DownloadID: "dl-002",
		Title:      "Movie.2024.REPACK.1080p",
	})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.DownloadID == "dl-002" && got.GrabTitle == "Movie.2024.REPACK.1080p"
	})

	got, _ := store.Get(ctx, wf.ID)
	if got.State != StateDownloading {
		t.Fatalf("expected state to remain downloading, got %s", got.State)
	}
}

func TestOrchestratorGrabbedResumesFromPostDownload(t *testing.T) {
	imp := &mockImporter{paths: []string{"/media/movie.mkv"}}
	orch, store := testOrchestrator(t, imp)
	ctx, _ := startOrchestrator(t, orch)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{"movie-457"})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}

	orch.Send(CmdGrabbed{
		WorkflowID: wf.ID,
		ClientID:   "qbit-1",
		DownloadID: "dl-101",
		Title:      "Movie.2024.1080p",
	})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StateDownloading
	})

	ok, err := store.Transition(ctx, wf.ID, StateDownloading, StatePostDownload, "test setup")
	if err != nil {
		t.Fatalf("transition to post_download: %v", err)
	}
	if !ok {
		t.Fatal("expected transition to post_download to succeed")
	}
	if _, err := store.IncrementRetry(ctx, wf.ID, "test retry"); err != nil {
		t.Fatalf("increment retry: %v", err)
	}
	if err := store.MergeMetadata(ctx, wf.ID, map[string]any{
		"status":       "seeding",
		"ratio":        1.75,
		"content_path": "/media/downloads/stale",
	}); err != nil {
		t.Fatalf("seed stale metadata: %v", err)
	}

	orch.Send(CmdGrabbed{
		WorkflowID: wf.ID,
		ClientID:   "qbit-1",
		DownloadID: "dl-102",
		Title:      "Movie.2024.REPACK.1080p",
	})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StateDownloading && got.DownloadID == "dl-102" && got.RetryCount == 0
	})

	got, _ := store.Get(ctx, wf.ID)
	if got.LastError != "" {
		t.Fatalf("expected last_error reset on re-grab, got %q", got.LastError)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(got.Metadata), &meta); err != nil {
		t.Fatalf("parse metadata: %v", err)
	}
	if status, _ := meta["status"].(string); status != "downloading" {
		t.Fatalf("expected status metadata to reset to downloading, got %q", status)
	}
	if cp, _ := meta["content_path"].(string); cp != "" {
		t.Fatalf("expected content_path to be cleared, got %q", cp)
	}
}

func TestOrchestratorDownloadComplete(t *testing.T) {
	imp := &mockImporter{paths: []string{"/media/movie.mkv"}}
	orch, store := testOrchestrator(t, imp)
	ctx, _ := startOrchestrator(t, orch)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{"movie-789"})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}

	// Grab
	orch.Send(CmdGrabbed{
		WorkflowID: wf.ID,
		ClientID:   "qbit-1",
		DownloadID: "dl-002",
		Title:      "Movie.2024.1080p",
	})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StateDownloading
	})

	// Download complete
	orch.Send(CmdDownloadComplete{
		ClientID:   "qbit-1",
		DownloadID: "dl-002",
		Title:      "Movie.2024.1080p",
		Category:   "movies",
	})

	// Should transition to post_download first
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StatePostDownload
	})

	// Backdate settling start so the delay is satisfied, then trigger evaluation.
	backdateSettling(t, store, ctx, wf.ID)

	// Send a progress update with status "completed" to trigger evaluation.
	orch.Send(CmdDownloadProgress{
		ClientID: "qbit-1", DownloadID: "dl-002",
		Progress: 1.0, Status: "completed",
	})

	// Should transition to importing and dispatch import
	waitForCondition(t, 3*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && (got.State == StateImporting || got.State == StateCompleted)
	})

	// Verify import was called
	waitForCondition(t, 2*time.Second, func() bool {
		return imp.callCount() > 0
	})

	imp.mu.Lock()
	defer imp.mu.Unlock()
	if len(imp.calls) != 1 {
		t.Fatalf("expected 1 import call, got %d", len(imp.calls))
	}
	call := imp.calls[0]
	if call.ClientID != "qbit-1" || call.DownloadID != "dl-002" {
		t.Fatalf("unexpected import call: %+v", call)
	}
}

func TestOrchestratorImportSuccess(t *testing.T) {
	imp := &mockImporter{paths: []string{"/media/movie.mkv"}}
	orch, store := testOrchestrator(t, imp)
	ctx, _ := startOrchestrator(t, orch)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{"movie-s1"})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}

	orch.Send(CmdGrabbed{
		WorkflowID: wf.ID,
		ClientID:   "qbit-1",
		DownloadID: "dl-s1",
		Title:      "Success.Movie",
	})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StateDownloading
	})

	orch.Send(CmdDownloadComplete{
		ClientID:   "qbit-1",
		DownloadID: "dl-s1",
		Title:      "Success.Movie",
		Category:   "movies",
	})

	// Wait for post_download, then backdate and trigger evaluation.
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StatePostDownload
	})
	backdateSettling(t, store, ctx, wf.ID)
	orch.Send(CmdDownloadProgress{
		ClientID: "qbit-1", DownloadID: "dl-s1",
		Progress: 1.0, Status: "completed",
	})

	// Import succeeds (mock returns paths, no error) → completed
	waitForCondition(t, 3*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StateCompleted
	})

	got, _ := store.Get(ctx, wf.ID)
	if got.State != StateCompleted {
		t.Fatalf("expected completed, got %s", got.State)
	}
}

func TestOrchestratorImportFailure(t *testing.T) {
	// Use "permission denied" to trigger failPermanent strategy (no delayed retry).
	imp := &mockImporter{err: fmt.Errorf("permission denied: /media/imports")}
	orch, store := testOrchestrator(t, imp)
	ctx, _ := startOrchestrator(t, orch)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{"movie-f1"})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}

	orch.Send(CmdGrabbed{
		WorkflowID: wf.ID,
		ClientID:   "qbit-1",
		DownloadID: "dl-f1",
		Title:      "Fail.Movie",
	})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StateDownloading
	})

	orch.Send(CmdDownloadComplete{
		ClientID:   "qbit-1",
		DownloadID: "dl-f1",
		Title:      "Fail.Movie",
		Category:   "movies",
	})

	// Wait for post_download, then backdate and trigger evaluation.
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StatePostDownload
	})
	backdateSettling(t, store, ctx, wf.ID)
	orch.Send(CmdDownloadProgress{
		ClientID: "qbit-1", DownloadID: "dl-f1",
		Progress: 1.0, Status: "completed",
	})

	// "permission denied" triggers failPermanent → markFailed is called immediately.
	waitForCondition(t, 3*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.LastError != ""
	})

	got, _ := store.Get(ctx, wf.ID)
	if got.LastError == "" {
		t.Fatal("expected last_error to be set")
	}
}

func TestClassifyImportError_MetadataNotResolvedIsPermanent(t *testing.T) {
	orch := &Orchestrator{}
	got := orch.classifyImportError(`download metadata not resolved yet: unresolved content path "/media/downloads/infohash:abc"`)
	if got != failPermanent {
		t.Fatalf("expected failPermanent, got %v", got)
	}
}

func TestClassifyImportError_DownloadPathNotFoundRetriesSearch(t *testing.T) {
	orch := &Orchestrator{}
	got := orch.classifyImportError(`download path not found: stat /media/downloads/Some.Release: no such file or directory`)
	if got != retrySearch {
		t.Fatalf("expected retrySearch, got %v", got)
	}
}

func TestOrchestratorCancel(t *testing.T) {
	imp := &mockImporter{paths: []string{"/media/movie.mkv"}}
	orch, store := testOrchestrator(t, imp)
	ctx, _ := startOrchestrator(t, orch)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{"movie-c1"})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}

	// Cancel via synchronous command
	reply := make(chan error, 1)
	orch.Send(CmdCancel{WorkflowID: wf.ID, Reply: reply})

	select {
	case err := <-reply:
		if err != nil {
			t.Fatalf("cancel returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancel reply")
	}

	got, _ := store.Get(ctx, wf.ID)
	if got.State != StateCancelled {
		t.Fatalf("expected cancelled, got %s", got.State)
	}
}

func TestOrchestratorProgressCoalescing(t *testing.T) {
	imp := &mockImporter{paths: []string{"/media/movie.mkv"}}
	orch, store := testOrchestrator(t, imp)
	ctx, _ := startOrchestrator(t, orch)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{"movie-p1"})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}

	orch.Send(CmdGrabbed{
		WorkflowID: wf.ID,
		ClientID:   "qbit-1",
		DownloadID: "dl-p1",
		Title:      "Progress.Movie",
	})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StateDownloading
	})

	// Send multiple progress updates — only latest should be buffered
	for i := 0; i < 10; i++ {
		orch.Send(CmdDownloadProgress{
			ClientID:   "qbit-1",
			DownloadID: "dl-p1",
			Progress:   float64(i) * 10,
			DownSpeed:  int64(i) * 1000,
		})
	}

	// Allow commands to be processed
	time.Sleep(100 * time.Millisecond)

	// Check progress buffer — only one entry per key
	orch.progressMu.Lock()
	key := "qbit-1:dl-p1"
	entry, exists := orch.progressBuf[key]
	orch.progressMu.Unlock()

	if !exists {
		t.Fatal("expected progress buffer to have entry")
	}
	if entry.Progress != 90 {
		t.Fatalf("expected latest progress 90, got %f", entry.Progress)
	}

	// Manually flush and verify metadata is updated
	orch.flushProgress(ctx)

	got, _ := store.Get(ctx, wf.ID)
	if got.Metadata == "" {
		t.Fatal("expected metadata to be set after flush")
	}
}

func TestOrchestratorEventLogging(t *testing.T) {
	imp := &mockImporter{paths: []string{"/media/movie.mkv"}}
	orch, store := testOrchestrator(t, imp)
	ctx, _ := startOrchestrator(t, orch)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{"movie-e1"})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}

	orch.Send(CmdGrabbed{
		WorkflowID: wf.ID,
		ClientID:   "qbit-1",
		DownloadID: "dl-e1",
		Title:      "Event.Movie",
	})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StateDownloading
	})

	events, err := store.ListEvents(ctx, wf.ID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected events to be logged")
	}

	// Should have at least: search_started, grabbed, downloading
	types := make(map[string]bool)
	for _, ev := range events {
		types[ev.EventType] = true
	}
	for _, expected := range []string{EventSearchStarted, EventGrabbed, EventDownloading} {
		if !types[expected] {
			t.Errorf("missing event type %s, got types: %v", expected, types)
		}
	}
}

func TestOrchestratorSeedingWaitsForRatio(t *testing.T) {
	ratioLimit := 1.5
	imp := &mockImporter{paths: []string{"/media/movie.mkv"}}
	orch, store := testOrchestrator(t, imp)
	ctx, _ := startOrchestrator(t, orch)

	wf, err := orch.StartSearch(ctx, TypeMovieSearch, MediaTypeMovie, "qp-1", []string{"movie-seed1"})
	if err != nil {
		t.Fatalf("StartSearch: %v", err)
	}

	// Grab with seed ratio requirement
	orch.Send(CmdGrabbed{
		WorkflowID:     wf.ID,
		ClientID:       "qbit-1",
		DownloadID:     "dl-seed1",
		Title:          "Seed.Movie",
		SeedRatioLimit: &ratioLimit,
	})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StateDownloading
	})

	// Download complete → post_download
	orch.Send(CmdDownloadComplete{
		ClientID: "qbit-1", DownloadID: "dl-seed1",
		Title: "Seed.Movie", Category: "movies",
	})
	waitForCondition(t, 2*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && got.State == StatePostDownload
	})

	// Backdate settling so only seed ratio matters.
	backdateSettling(t, store, ctx, wf.ID)

	// Send seeding progress with ratio below limit — should NOT transition.
	orch.Send(CmdDownloadProgress{
		ClientID: "qbit-1", DownloadID: "dl-seed1",
		Progress: 1.0, Ratio: 0.5, Status: "seeding",
	})
	time.Sleep(100 * time.Millisecond)

	got, _ := store.Get(ctx, wf.ID)
	if got.State != StatePostDownload {
		t.Fatalf("expected post_download while seeding below ratio, got %s", got.State)
	}

	// Now send progress with ratio meeting the limit.
	orch.Send(CmdDownloadProgress{
		ClientID: "qbit-1", DownloadID: "dl-seed1",
		Progress: 1.0, Ratio: 1.6, Status: "seeding",
	})
	waitForCondition(t, 3*time.Second, func() bool {
		got, _ := store.Get(ctx, wf.ID)
		return got != nil && (got.State == StateImporting || got.State == StateCompleted)
	})
}
