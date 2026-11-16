package workflows

import (
	"context"
	"testing"
	"time"
)

func TestAuthorizedReacquisitionRearmsFailedImportWithSameDownloadIdentity(t *testing.T) {
	ctx := context.Background()
	imp := &mockImporter{paths: []string{"/music/Artist/Album/01.flac"}}
	orch, store := testOrchestrator(t, imp)
	failed := &Workflow{
		Type: TypeManualImport, State: StateFailed, GrabTitle: "Artist - Album",
		DownloadClientID: "client", DownloadID: "same-torrent",
		MaxRetries: MaxRetries, RetryCount: MaxRetries,
		LastError: "Import failed: album track list unavailable",
		Metadata:  `{"failed_from":"importing","category":"music","content_path":"/downloads/old"}`,
	}
	if err := store.Create(ctx, failed); err != nil {
		t.Fatal(err)
	}
	completion := CmdDownloadComplete{
		ClientID: "client", DownloadID: "same-torrent", Title: failed.GrabTitle,
		Category: "music", ContentPath: "/downloads/new",
	}
	orch.handleDownloadComplete(ctx, completion)
	if got := mustGet(t, store, ctx, failed.ID); got.State != StateFailed {
		t.Fatalf("ordinary completion must not loop on a failed import: %+v", got)
	}
	reply := make(chan error, 1)
	orch.handle(ctx, CmdDownloadReacquired{ClientID: "client", DownloadID: "same-torrent", Reply: reply})
	if err := <-reply; err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, store, ctx, failed.ID); got.State != StateDownloading || got.RetryCount != 0 {
		t.Fatalf("authorized retry did not re-arm failed import: %+v", got)
	}
	orch.handleDownloadComplete(ctx, completion)
	orch.handleDownloadComplete(ctx, completion)
	wf := mustGet(t, store, ctx, failed.ID)
	if wf.State != StatePostDownload || metadataPathFromWorkflow(wf.Metadata) != "/downloads/new" {
		t.Fatalf("same-identity completion suppressed or lost latest path: %+v", wf)
	}
	backdateSettling(t, store, ctx, wf.ID)
	wf = mustGet(t, store, ctx, wf.ID)
	orch.evaluatePostDownload(ctx, wf, 0, "completed")
	select {
	case cmd := <-orch.commands:
		result, ok := cmd.(CmdImportResult)
		if !ok || !result.Success || result.WorkflowID != wf.ID {
			t.Fatalf("re-acquired music was not dispatched: %#v", cmd)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("same download was not re-imported")
	}
	imp.mu.Lock()
	defer imp.mu.Unlock()
	if len(imp.calls) != 1 || imp.calls[0].DownloadID != "same-torrent" || imp.calls[0].Category != "music" {
		t.Fatalf("recovery lost identity or duplicated dispatch: %+v", imp.calls)
	}
}

func TestReacquisitionDoesNotDisturbActiveOrCompletedWorkflows(t *testing.T) {
	for _, state := range []string{StateImporting, StateCompleted} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			orch, store := testOrchestrator(t, &mockImporter{})
			wf := &Workflow{
				Type: TypeManualImport, State: state,
				DownloadClientID: "client", DownloadID: "download",
			}
			if err := store.Create(ctx, wf); err != nil {
				t.Fatal(err)
			}
			if err := orch.resumeReacquiredDownload(ctx, "client", "download"); err != nil {
				t.Fatal(err)
			}
			if got := mustGet(t, store, ctx, wf.ID); got.State != state {
				t.Fatalf("re-acquisition disturbed %s workflow: %+v", state, got)
			}
		})
	}
}

func TestResumeReacquiredDownloadUsesCommandChannel(t *testing.T) {
	orch, _ := testOrchestrator(t, &mockImporter{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go orch.Run(ctx)
	if err := orch.ResumeReacquiredDownload(ctx, "client", "new-download"); err != nil {
		t.Fatal(err)
	}
	if err := orch.ResumeReacquiredDownload(ctx, "", ""); err == nil {
		t.Fatal("empty identity accepted")
	}
}
