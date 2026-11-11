package imports

import (
	"context"
	"encoding/json"
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

	"github.com/ebenderooock/loom/internal/series"
)

type scanSeriesGate struct {
	*packSeriesService
	blockAt int32
	calls   atomic.Int32
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (g *scanSeriesGate) SearchSeries(ctx context.Context, query string) ([]*series.Series, error) {
	call := g.calls.Add(1)
	if g.blockAt == 0 || call == g.blockAt {
		g.once.Do(func() { close(g.entered) })
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-g.release:
		}
	}
	return g.packSeriesService.SearchSeries(ctx, query)
}

func scanHTTP(t *testing.T, router http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("%s %s waited for filesystem work", method, target)
	}
	return rec
}

func scanResponse(t *testing.T, rec *httptest.ResponseRecorder) ImportScan {
	t.Helper()
	var response struct {
		Data ImportScan `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response.Data
}

func awaitFolderScan(t *testing.T, p *ImportPipeline, id string) *ImportScan {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		scan := p.GetFolderScan(id, 0, 1000)
		if scan != nil && scan.Status != "running" {
			return scan
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("folder scan did not finish")
	return nil
}

func TestAsyncFolderScanRespondsBeforeMatchingAndPagesProgress(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	t.Cleanup(f.p.Stop)
	dir, files := writePack(t, t.TempDir(), 3)
	gate := &scanSeriesGate{
		packSeriesService: f.svc, blockAt: 2,
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	f.p.matcher.seriesSvc = gate
	defer close(gate.release)
	body, _ := json.Marshal(map[string]string{"path": dir})
	router := Router(f.p)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/scans", strings.NewReader(string(body))).WithContext(ctx))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scan submission waited for slow matching instead of returning promptly")
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	started := scanResponse(t, rec)
	if len(started.Results) != 0 || started.Status != "running" {
		t.Fatalf("submission claimed completed work: %+v", started)
	}
	cancel() // Ending the request must not cancel the accepted background scan.
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not survive request cancellation")
	}
	progress := scanResponse(t, scanHTTP(t, router, http.MethodGet, "/scans/"+started.ID+"?limit=1", ""))
	if progress.Status != "running" || progress.Phase != "matching" ||
		progress.TotalFiles != 3 || progress.ProcessedFiles != 1 || progress.NextOffset != 1 ||
		len(progress.Results) != 1 || progress.Results[0].FilePath != files[0] {
		t.Fatalf("partial scan progress: %+v", progress)
	}
	duplicate := scanResponse(t, scanHTTP(t, router, http.MethodPost, "/scans", string(body)))
	if duplicate.ID != started.ID {
		t.Fatal("duplicate submission started another filesystem scan")
	}
	gate.release <- struct{}{}
	finished := awaitFolderScan(t, f.p, started.ID)
	if finished.Status != "completed" || finished.ProcessedFiles != 3 {
		t.Fatalf("completed scan: %+v", finished)
	}
	page := scanResponse(t, scanHTTP(t, router, http.MethodGet, "/scans/"+started.ID+"?offset=1&limit=1", ""))
	if len(page.Results) != 1 || page.Results[0].FilePath != files[1] ||
		page.NextOffset != 2 || page.ProcessedFiles != 3 {
		t.Fatalf("completed results cursor: %+v", page)
	}
	last := scanResponse(t, scanHTTP(t, router, http.MethodGet, "/scans/"+started.ID+"?offset=2&limit=1", ""))
	if len(last.Results) != 1 || last.Results[0].FilePath != files[2] || last.NextOffset != 3 {
		t.Fatalf("last results cursor: %+v", last)
	}
}

func TestAsyncFolderScanCancellationCapacityAndShutdown(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	t.Cleanup(f.p.Stop)
	gate := &scanSeriesGate{
		packSeriesService: f.svc, entered: make(chan struct{}), release: make(chan struct{}),
	}
	f.p.matcher.seriesSvc = gate
	router := Router(f.p)
	dir, _ := writePack(t, t.TempDir(), 1)
	otherDir, _ := writePack(t, t.TempDir(), 1)
	started, err := f.p.StartFolderScan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not enter matching")
	}
	if _, err := f.p.StartFolderScan(context.Background(), otherDir); err != nil {
		t.Fatal(err)
	}
	rec := scanHTTP(t, router, http.MethodPost, "/scans", `{"path":"/third-scan"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("unbounded concurrent scans: %d %s", rec.Code, rec.Body)
	}
	rec = scanHTTP(t, router, http.MethodDelete, "/scans/"+started.ID, "")
	if rec.Code != http.StatusOK || scanResponse(t, rec).Status != "cancelled" {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	f.p.Stop()
	scan := f.p.GetFolderScan(started.ID, 0, 1000)
	if scan.Status != "cancelled" || scan.ProcessedFiles != 0 {
		t.Fatalf("cancelled scan published unverified results: %+v", scan)
	}
	rec = scanHTTP(t, router, http.MethodPost, "/scans", `{"path":"/after-shutdown"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("scan accepted after shutdown: %d", rec.Code)
	}
}

func TestAsyncFolderScanFailureExpiryAndRetention(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	t.Cleanup(f.p.Stop)
	router := Router(f.p)
	for _, body := range []string{`{`, `{}`, `{"path":"   "}`} {
		if rec := scanHTTP(t, router, http.MethodPost, "/scans", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid scan accepted: %d %s", rec.Code, rec.Body)
		}
	}
	started, err := f.p.StartFolderScan(context.Background(), filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	failed := awaitFolderScan(t, f.p, started.ID)
	if failed.Status != "failed" || !strings.Contains(failed.Error, "inspect import path") || len(failed.Results) != 0 {
		t.Fatalf("filesystem failure not surfaced: %+v", failed)
	}
	for _, query := range []string{"offset=-1", "offset=nope", "limit=0", "limit=-1"} {
		rec := scanHTTP(t, router, http.MethodGet, "/scans/"+started.ID+"?"+query, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid cursor accepted: %s: %d", query, rec.Code)
		}
	}
	f.p.scans.mu.Lock()
	f.p.scans.tasks[started.ID].scan.UpdatedAt = time.Now().Add(-11 * time.Minute)
	f.p.scans.mu.Unlock()
	if rec := scanHTTP(t, router, http.MethodGet, "/scans/"+started.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("expired scan still returned: %d", rec.Code)
	}
	var first, last string
	for range 10 {
		started, err := f.p.StartFolderScan(context.Background(), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = started.ID
		}
		last = started.ID
		awaitFolderScan(t, f.p, started.ID)
	}
	if f.p.GetFolderScan(first, 0, 1) != nil || f.p.GetFolderScan(last, 0, 1) == nil {
		t.Fatal("scan retention failed to evict the oldest finished scan")
	}
}

func TestContentVerificationCancellationDoesNotCacheAnUnverifiedResult(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	dir := t.TempDir()
	src, dest := filepath.Join(dir, "source.mkv"), filepath.Join(dir, "dest.mkv")
	for _, path := range []string{src, dest} {
		if err := os.WriteFile(path, []byte("identical bytes, distinct files"), 0o600); err != nil {
			t.Fatal(err)
		}

	}
	srcInfo, _ := os.Stat(src)
	destInfo, _ := os.Stat(dest)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if same, err := f.p.sameContent(ctx, src, dest, srcInfo, destInfo); same || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled verification: %v, %v", same, err)
	}
	if same, err := f.p.sameContent(context.Background(), src, dest, srcInfo, destInfo); !same || err != nil {
		t.Fatalf("cancelled hash poisoned later verification: %v, %v", same, err)
	}
}

func TestAsyncFolderScanBoundsLargeResponsesWithoutLosingResults(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	t.Cleanup(f.p.Stop)
	results := make([]ScanResult, 1002)
	f.p.scans.tasks["large"] = &scanTask{
		finished: true,
		scan: ImportScan{
			ID: "large", Status: "completed", ProcessedFiles: len(results),
			TotalFiles: len(results), UpdatedAt: time.Now(), Results: results,
		},
	}
	router := Router(f.p)
	first := scanResponse(t, scanHTTP(t, router, http.MethodGet, "/scans/large?limit=2000", ""))
	if len(first.Results) != 1000 || first.NextOffset != 1000 || first.ProcessedFiles != 1002 {
		t.Fatalf("unbounded result response: %d results, offset %d", len(first.Results), first.NextOffset)
	}
	last := scanResponse(t, scanHTTP(t, router, http.MethodGet, "/scans/large?offset=1000", ""))
	if len(last.Results) != 2 || last.NextOffset != first.ProcessedFiles {
		t.Fatalf("remaining results lost after completion: %d results, offset %d", len(last.Results), last.NextOffset)
	}
}
