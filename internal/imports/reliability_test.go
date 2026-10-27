package imports

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/safety"
	"github.com/ebenderooock/loom/internal/workflows"
)

func TestCompletionLeavesTrackedDownloadsToOrchestrator(t *testing.T) {
	opts := basePipelineOptions(t)
	store, err := workflows.NewStore(opts.DB)
	if err != nil {
		t.Fatal(err)
	}
	wf := &workflows.Workflow{
		Type: workflows.TypeMovieSearch, State: workflows.StateDownloading,
		MediaType: workflows.MediaTypeMovie, DownloadClientID: "client", DownloadID: "download",
	}
	if err := store.Create(context.Background(), wf); err != nil {
		t.Fatal(err)
	}
	opts.WorkflowEngine = workflows.NewEngine(store, nil, opts.Logger)
	p, err := NewPipeline(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.handleCompleted(context.Background(), &downloads.DownloadCompletedEvent{
		ClientID: "client", DownloadID: "download", Title: "Tracked",
	}); err != nil {
		t.Fatal(err)
	}
	var attempts int
	if err := p.db.QueryRow(`SELECT COUNT(*) FROM import_history`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Fatalf("event handler attempted an orchestrator-owned import: %d attempts", attempts)
	}
}

func TestMoveKeepsSourceUntilLibraryRegistrationSucceeds(t *testing.T) {
	p, svc, destDir := newCollisionPipeline(t, ImportModeMove)
	src := filepath.Join(t.TempDir(), "Safety.Film.2026.mkv")
	sub := strings.TrimSuffix(src, ".mkv") + ".srt"
	for _, path := range []string{src, sub} {
		if err := os.WriteFile(path, []byte("downloaded content"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	svc.err = errors.New("database temporarily unavailable")
	ev := &downloads.DownloadCompletedEvent{Title: "Safety Film 2026"}
	if _, err := p.importSingleFile(context.Background(), ev, src); err == nil {
		t.Fatal("expected library failure")
	}
	for _, path := range []string{src, sub} {
		if got, err := os.ReadFile(path); err != nil || string(got) != "downloaded content" {
			t.Fatalf("failed registration consumed source %s: %q, %v", path, got, err)
		}
	}
	dest := filepath.Join(destDir, "Safety Film (2026).mkv")
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("failed attempt left a conflicting staged destination: %v", err)
	}

	svc.err = nil
	if got, err := p.importSingleFile(context.Background(), ev, src); err != nil || got != dest {
		t.Fatalf("retry failed: %q, %v", got, err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("successful move did not remove source: %v", err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "downloaded content" {
		t.Fatalf("retry lost file contents: %q, %v", got, err)
	}
}

func TestVerificationFailurePreservesMoveSource(t *testing.T) {
	p, _, destDir := newCollisionPipeline(t, ImportModeMove)
	src := filepath.Join(t.TempDir(), "Safety.Film.2026.mkv")
	if err := os.WriteFile(src, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := p.importSingleFile(context.Background(), &downloads.DownloadCompletedEvent{Title: "Safety Film 2026"}, src)
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("expected verification failure, got %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("verification consumed the source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "Safety Film (2026).mkv")); !os.IsNotExist(err) {
		t.Fatalf("unverified destination was not removed: %v", err)
	}
}

func TestPartialDownloadImportDoesNotReportSuccess(t *testing.T) {
	p, _, destDir := newCollisionPipeline(t, ImportModeCopy)
	srcDir := t.TempDir()
	for _, name := range []string{"Safety.Film.2026.mkv", "Safety.Film.2026.mp4"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte("incoming"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	collision := filepath.Join(destDir, "Safety Film (2026).mp4")
	if err := os.WriteFile(collision, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	ev := &downloads.DownloadCompletedEvent{DownloadID: "download", Title: "Safety Film 2026"}
	paths, err := p.processImport(context.Background(), ev, srcDir)
	if !errors.Is(err, ErrDestinationExists) || len(paths) != 1 {
		t.Fatalf("partial import must return both progress and failure: %v, %v", paths, err)
	}
	if got, err := os.ReadFile(collision); err != nil || string(got) != "existing" {
		t.Fatalf("partial failure changed existing media: %q, %v", got, err)
	}
	// Retrying must recognize the successful copy instead of rejecting it.
	paths, err = p.processImport(context.Background(), ev, srcDir)
	if !errors.Is(err, ErrDestinationExists) || len(paths) != 1 {
		t.Fatalf("partial retry lost successful progress: %v, %v", paths, err)
	}
}

func TestCompletedImportsAreIdempotentButDoNotHideChangedFiles(t *testing.T) {
	for _, mode := range []ImportMode{ImportModeCopy, ImportModeHardlink} {
		t.Run(string(mode), func(t *testing.T) {
			p, svc, _ := newCollisionPipeline(t, mode)
			src := filepath.Join(t.TempDir(), "Safety.Film.2026.mkv")
			if err := os.WriteFile(src, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			ev := &downloads.DownloadCompletedEvent{Title: "Safety Film 2026"}
			dest, err := p.importSingleFile(context.Background(), ev, src)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.importSingleFile(context.Background(), ev, src)
			if err != nil || got != dest || len(svc.files) != 1 {
				t.Fatalf("retry duplicated successful import: %q, %v, %d records", got, err, len(svc.files))
			}
			// Replace the inode so hard-link destinations remain unchanged too.
			if err := os.Remove(src); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(src, []byte("modified"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := p.importSingleFile(context.Background(), ev, src); !errors.Is(err, ErrDestinationExists) {
				t.Fatalf("same-size different content mistaken for completed import: %v", err)
			}
			if err := os.Remove(dest); err != nil {
				t.Fatal(err)
			}
			if _, err := p.importSingleFile(context.Background(), ev, src); err != nil {
				t.Fatalf("missing destination prevented recovery: %v", err)
			}
		})
	}
}

func TestRunImportReturnsLibraryDestinations(t *testing.T) {
	p, _, destDir := newCollisionPipeline(t, ImportModeCopy)
	src := filepath.Join(t.TempDir(), "Safety.Film.2026.MKV")
	if err := os.WriteFile(src, []byte("download"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.downloadSvc.Registry().Register(&resolvePathFakeClient{
		id: "client", items: []downloads.Item{{ID: "download", ContentPath: src}},
	}); err != nil {
		t.Fatal(err)
	}
	paths, err := p.RunImport(context.Background(), "client", "download", "Safety Film 2026", "")
	if err != nil || len(paths) != 1 || paths[0] != filepath.Join(destDir, "Safety Film (2026).MKV") {
		t.Fatalf("expected library destination, got %v, %v", paths, err)
	}
}

func TestSingleNonMediaDownloadNeverImportsSiblingFiles(t *testing.T) {
	p, svc, _ := newCollisionPipeline(t, ImportModeMove)
	dir := t.TempDir()
	for _, name := range []string{"download.txt", "Safety.Film.2026.mkv"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("keep me"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.processImport(context.Background(), &downloads.DownloadCompletedEvent{
		Title: "Safety Film 2026",
	}, filepath.Join(dir, "download.txt")); err == nil {
		t.Fatal("non-media download reported success")
	}
	if len(svc.files) != 0 {
		t.Fatal("imported unrelated sibling file")
	}
}

func TestScanMediaFilesHandlesUppercaseAndReportsMissingRoots(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Movie.MKV")
	if err := os.WriteFile(path, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := scanMediaFiles(root)
	if err != nil || len(files) != 1 || files[0] != path {
		t.Fatalf("uppercase extension not scanned: %v, %v", files, err)
	}
	if _, err := scanMediaFiles(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing scan root was silently ignored")
	}
}

func TestSampleDoesNotMakeAnOtherwiseCompleteBatchFail(t *testing.T) {
	p, _, _ := newCollisionPipeline(t, ImportModeHardlink)
	p.decisions = NewDecisionMaker(&SampleSpec{})
	root := t.TempDir()
	main := filepath.Join(root, "Safety.Film.2026.mkv")
	sample := filepath.Join(root, "Safety.Film.2026.sample.mp4")
	for _, path := range []string{main, sample} {
		if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Sparse source meets the real movie-size threshold without copying data.
	if err := os.Truncate(main, 100*1024*1024); err != nil {
		t.Fatal(err)
	}
	paths, err := p.processImport(context.Background(), &downloads.DownloadCompletedEvent{
		Title: "Safety Film 2026", DownloadID: "download",
	}, root)
	if err != nil || len(paths) != 1 {
		t.Fatalf("sample caused false batch failure: %v, %v", paths, err)
	}
	if _, err := os.Stat(sample); err != nil {
		t.Fatalf("skipped sample was consumed: %v", err)
	}
}

func TestDownloadMovesCanResumeBeforeSourceFinalization(t *testing.T) {
	p, svc, _ := newCollisionPipeline(t, ImportModeMove)
	src := filepath.Join(t.TempDir(), "Safety.Film.2026.mkv")
	if err := os.WriteFile(src, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	ev := &downloads.DownloadCompletedEvent{Title: "Safety Film 2026", DownloadID: "download"}
	paths, err := p.processImport(context.Background(), ev, src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source consumed before durable workflow result: %v", err)
	}
	retried, err := p.processImport(context.Background(), ev, src)
	if err != nil || len(retried) != 1 || retried[0] != paths[0] || len(svc.files) != 1 {
		t.Fatalf("interrupted import could not resume: %v, %v", retried, err)
	}
	if err := p.FinalizeImports(context.Background(), paths); err != nil {
		t.Fatal(err)
	}
	if err := p.FinalizeImports(context.Background(), paths); err != nil {
		t.Fatalf("finalization must be repeatable after restart: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("committed move retained source: %v", err)
	}
	if got, err := os.ReadFile(paths[0]); err != nil || string(got) != "original" {
		t.Fatalf("finalization damaged imported media: %q, %v", got, err)
	}
}

func TestFinalizationDoesNotDeleteReplacedSource(t *testing.T) {
	p, _, _ := newCollisionPipeline(t, ImportModeMove)
	src := filepath.Join(t.TempDir(), "Safety.Film.2026.mkv")
	if err := os.WriteFile(src, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err := p.processImport(context.Background(), &downloads.DownloadCompletedEvent{
		Title: "Safety Film 2026", DownloadID: "download",
	}, src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("modified"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.FinalizeImports(context.Background(), paths); err == nil {
		t.Fatal("finalization did not detect a replaced source")
	}
	if got, err := os.ReadFile(src); err != nil || string(got) != "modified" {
		t.Fatalf("finalization deleted changed source: %q, %v", got, err)
	}
}

func TestReviewRequiredDownloadDoesNotReportImportSuccess(t *testing.T) {
	p, svc, _ := newCollisionPipeline(t, ImportModeMove)
	p.postVal = safety.NewPostValidator(safety.ReleaseValidatorConfig{})
	root := t.TempDir()
	for _, name := range []string{"Safety.Film.2026.mkv", "suspicious.exe"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := p.processImport(context.Background(), &downloads.DownloadCompletedEvent{
		Title: "Safety Film 2026", DownloadID: "download",
	}, root)
	if err == nil || !strings.Contains(err.Error(), "pending review") || len(paths) != 0 || len(svc.files) != 0 {
		t.Fatalf("review-required download reported success: %v, %v", paths, err)
	}
}
