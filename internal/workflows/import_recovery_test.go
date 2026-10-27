package workflows

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOrphanCompletionGetsRecoverableWorkflow(t *testing.T) {
	orch, store := testOrchestrator(t, &mockImporter{})
	ctx := context.Background()
	cmd := CmdDownloadComplete{
		ClientID: "client", DownloadID: "orphan", Title: "Movie",
		ContentPath: "/downloads/Movie", Category: "movies",
	}
	orch.handleDownloadComplete(ctx, cmd)
	orch.handleDownloadComplete(ctx, cmd)
	wf, err := store.FindByDownload(ctx, "client", "orphan")
	if err != nil || wf == nil {
		t.Fatalf("orphan was not tracked: %v", err)
	}
	if wf.State != StatePostDownload || wf.MaxRetries != MaxRetries ||
		metadataPathFromWorkflow(wf.Metadata) != "/downloads/Movie" {
		t.Fatalf("orphan bypassed settling or lost recovery data: %+v", wf)
	}
}

func TestBootRecoveryReplaysPersistedCleanupDestinations(t *testing.T) {
	orch, store := testOrchestrator(t, &mockImporter{})
	ctx := context.Background()
	wf := &Workflow{
		Type: TypeMovieSearch, State: StateCleaningUp, MediaType: MediaTypeMovie,
		DownloadClientID: "client", DownloadID: "download",
		Metadata: `{"content_path":"/downloads/Movie","imported_paths":["/library/Movie.mkv"]}`,
	}
	if err := store.Create(ctx, wf); err != nil {
		t.Fatal(err)
	}
	cleaned := make(chan []string, 1)
	orch.cleanupFn = func(_ context.Context, _, _, _ string, paths []string) error {
		cleaned <- paths
		return nil
	}
	orch.reconcileOnBoot(ctx)
	select {
	case paths := <-cleaned:
		if len(paths) != 1 || paths[0] != "/library/Movie.mkv" {
			t.Fatalf("restart discarded cleanup destinations: %v", paths)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restart did not resume cleanup")
	}
	waitForCondition(t, time.Second, func() bool {
		got, err := store.Get(ctx, wf.ID)
		return err == nil && got.State == StateCompleted
	})
}

func TestRetryFailedImportUsesCachedFilesWithoutDownloadClient(t *testing.T) {
	imp := &mockImporter{paths: []string{"/library/Movie.mkv"}}
	orch, store := testOrchestrator(t, imp)
	orch.dlStatus = nil
	ctx := context.Background()
	wf := &Workflow{
		Type: TypeMovieSearch, State: StateFailed, MediaType: MediaTypeMovie,
		DownloadClientID: "client", DownloadID: "removed", GrabTitle: "Movie",
		LastError: "Import transient error: download path not found",
		Metadata:  `{"content_path":"/downloads/Movie","category":"movies"}`,
	}
	if err := store.Create(ctx, wf); err != nil {
		t.Fatal(err)
	}
	reply := make(chan error, 1)
	orch.handleRetry(ctx, CmdRetry{WorkflowID: wf.ID, Reply: reply})
	if err := <-reply; err != nil {
		t.Fatal(err)
	}
	select {
	case cmd := <-orch.commands:
		result, ok := cmd.(CmdImportResult)
		if !ok || !result.Success {
			t.Fatalf("cached retry did not run import: %#v", cmd)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cached retry did not dispatch import")
	}
	if got := mustGet(t, store, ctx, wf.ID); got.State != StateImporting {
		t.Fatalf("cached import incorrectly restarted search: %s", got.State)
	}
	imp.mu.Lock()
	defer imp.mu.Unlock()
	if len(imp.calls) != 1 || imp.calls[0].DownloadID != "removed" || imp.calls[0].Category != "movies" {
		t.Fatalf("retry lost original download identity: %+v", imp.calls)
	}
}

func TestPreImportCachesPathsEvenWhenPreparationFails(t *testing.T) {
	imp := &mockImporter{}
	orch, store := testOrchestrator(t, imp)
	ctx := context.Background()
	wf := &Workflow{
		Type: TypeMovieSearch, State: StateImporting, MediaType: MediaTypeMovie,
		DownloadClientID: "client", DownloadID: "download",
	}
	if err := store.Create(ctx, wf); err != nil {
		t.Fatal(err)
	}
	orch.SetPreImportFn(func(context.Context, string, string) (string, string, error) {
		return "/downloads/Movie", "/downloads", errors.New("client pause timed out")
	})
	orch.dispatchImport(ctx, wf.ID, "client", "download", "Movie", "")
	select {
	case <-orch.commands:
	case <-time.After(5 * time.Second):
		t.Fatal("import not dispatched")
	}
	got := mustGet(t, store, ctx, wf.ID)
	if metadataPathFromWorkflow(got.Metadata) != "/downloads/Movie" {
		t.Fatalf("preparation failure discarded resolved paths: %s", got.Metadata)
	}
}

func TestImportRejectionFailsWithoutCleanup(t *testing.T) {
	orch, store := testOrchestrator(t, &mockImporter{})
	ctx := context.Background()
	wf := &Workflow{Type: TypeMovieSearch, State: StateImporting, MediaType: MediaTypeMovie}
	if err := store.Create(ctx, wf); err != nil {
		t.Fatal(err)
	}
	orch.handleImportResult(ctx, CmdImportResult{WorkflowID: wf.ID, Error: "import pending review: dangerous file"})
	got := mustGet(t, store, ctx, wf.ID)
	if got.State != StateFailed || !strings.Contains(got.LastError, "pending review") {
		t.Fatalf("rejection left workflow active or lost failure: %+v", got)
	}
}

func TestTemporaryImportFailuresDoNotTriggerNewDownloads(t *testing.T) {
	orch := &Orchestrator{}
	for _, message := range []string{
		"download metadata not resolved yet: unresolved content path",
		"download path not found: no such file or directory",
		"no media files found in /downloads/still-unpacking",
		"scan media files: input/output error",
		"verification failed: size mismatch",
		"library update failed: database locked",
	} {
		t.Run(message, func(t *testing.T) {
			if got := orch.classifyImportError(message); got != retryImport {
				t.Fatalf("temporary failure must retry existing download, got %v", got)
			}
		})
	}
}
