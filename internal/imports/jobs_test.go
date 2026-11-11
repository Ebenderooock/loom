package imports

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/safety"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/workflows"
)

// packSeriesService is a minimal series backend: one show, one season with
// three episodes, recording created episode files.
type packSeriesService struct {
	series.Service
	mu    sync.Mutex
	show  *series.Series
	eps   []*series.Episode
	files []*series.EpisodeFile
	fail  map[string]error // episode ID -> CreateEpisodeFile error
}

func (s *packSeriesService) SearchSeries(context.Context, string) ([]*series.Series, error) {
	return []*series.Series{s.show}, nil
}
func (s *packSeriesService) GetSeries(context.Context, string) (*series.Series, error) {
	return s.show, nil
}
func (s *packSeriesService) ListEpisodes(_ context.Context, _ string, season *int) ([]*series.Episode, error) {
	if season != nil && *season != 1 {
		return nil, nil
	}
	return s.eps, nil
}
func (s *packSeriesService) GetEpisode(_ context.Context, id string) (*series.Episode, error) {
	for _, e := range s.eps {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, errors.New("episode not found")
}
func (s *packSeriesService) ListSeasons(context.Context, string) ([]*series.Season, error) {
	return []*series.Season{{ID: "season-1", SeriesID: s.show.ID, SeasonNumber: 1}}, nil
}
func (s *packSeriesService) CreateEpisodeFile(_ context.Context, f *series.EpisodeFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.fail[f.EpisodeID]; err != nil {
		return err
	}
	s.files = append(s.files, f)
	return nil
}
func (s *packSeriesService) episodeFiles() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for _, f := range s.files {
		out[f.EpisodeID] = f.FilePath
	}
	return out
}

type noMoviesService struct{ movies.Service }

func (noMoviesService) SearchMovies(context.Context, string) ([]*movies.Movie, error) {
	return nil, nil
}

type packFixture struct {
	p    *ImportPipeline
	svc  *packSeriesService
	opts PipelineOptions
	lib  string
}

func newPackPipeline(t *testing.T, mode ImportMode, mutate func(*PipelineOptions)) *packFixture {
	t.Helper()
	opts := basePipelineOptions(t)
	lib := t.TempDir()
	if _, err := opts.DB.Exec(`CREATE TABLE libraries (
		id TEXT PRIMARY KEY, name TEXT, path TEXT, media_type TEXT,
		monitor_on_add INTEGER, quality_profile_id TEXT,
		unmonitor_on_delete INTEGER, auto_archive_watched INTEGER,
		auto_archive_days_after_watch INTEGER, created_at DATETIME, updated_at DATETIME
	)`); err != nil {
		t.Fatal(err)
	}
	opts.LibStore = libraries.NewStore(opts.DB)
	if err := opts.LibStore.Create(context.Background(), &libraries.Library{
		ID: "tv", Name: "TV", Path: lib, MediaType: "series",
	}); err != nil {
		t.Fatal(err)
	}
	svc := &packSeriesService{
		show: &series.Series{ID: "show", Title: "Supernatural", Year: 2005, LibraryID: "tv"},
		fail: map[string]error{},
	}
	for i := 1; i <= 3; i++ {
		svc.eps = append(svc.eps, &series.Episode{ID: fmt.Sprintf("ep%d", i), SeriesID: "show", EpisodeNumber: i})
	}
	opts.SeriesSvc = svc
	opts.MoviesSvc = noMoviesService{}
	opts.ImportMode = mode
	if mutate != nil {
		mutate(&opts)
	}
	p, err := NewPipeline(opts)
	if err != nil {
		t.Fatal(err)
	}
	p.decisions = NewDecisionMaker()
	p.jobs.backoff = []time.Duration{time.Millisecond}
	return &packFixture{p: p, svc: svc, opts: opts, lib: lib}
}

// writePack creates a season-pack folder with one file per episode.
func writePack(t *testing.T, root string, n int) (string, []string) {
	t.Helper()
	dir := filepath.Join(root, "Supernatural.S01.1080p.WEB-DL")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var files []string
	for i := 1; i <= n; i++ {
		f := filepath.Join(dir, fmt.Sprintf("Supernatural.S01E%02d.1080p.WEB-DL.mkv", i))
		if err := os.WriteFile(f, []byte(fmt.Sprintf("episode %d", i)), 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	return dir, files
}

// drain claims and runs runnable jobs until none remain.
func drain(t *testing.T, p *ImportPipeline) {
	t.Helper()
	for i := 0; i < 50; i++ {
		job, err := p.claimJob(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if job == nil {
			return
		}
		p.runJob(context.Background(), job)
	}
	t.Fatal("job queue did not drain")
}

func mustJob(t *testing.T, p *ImportPipeline, id string) *ImportJob {
	t.Helper()
	j, err := p.GetJob(context.Background(), id, true)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestSeasonPackGrabbedForOneEpisodeImportsEveryEpisode(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, func(o *PipelineOptions) {
		store, err := workflows.NewStore(o.DB)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Create(context.Background(), &workflows.Workflow{
			Type: workflows.TypeEpisodeSearch, State: workflows.StateDownloading, MediaType: workflows.MediaTypeEpisode,
			DownloadClientID: "client", DownloadID: "pack",
			Items: []workflows.Item{{MediaType: workflows.MediaTypeEpisode, MediaID: "ep1"}},
		}); err != nil {
			t.Fatal(err)
		}
		o.WorkflowEngine = workflows.NewEngine(store, nil, o.Logger)
	})
	_, files := writePack(t, t.TempDir(), 3)
	ev := &downloads.DownloadCompletedEvent{ClientID: "client", DownloadID: "pack", Title: "Supernatural.S01.1080p.WEB-DL"}
	for _, file := range files {
		if _, err := f.p.importDownloadFile(context.Background(), ev, file, len(files)); err != nil {
			t.Fatalf("import %s: %v", file, err)
		}
	}
	got := f.svc.episodeFiles()
	for i := 1; i <= 3; i++ {
		if got[fmt.Sprintf("ep%d", i)] == "" {
			t.Fatalf("episode %d not imported; files=%v", i, got)
		}
	}
}

func TestEnqueueAcceptsMoreThanTwoJobsAndProcessesAll(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	root := t.TempDir()
	var ids []string
	for i := 1; i <= 3; i++ {
		file := filepath.Join(root, fmt.Sprintf("Supernatural.S01E%02d.mkv", i))
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		job, err := f.p.EnqueueImport(context.Background(), JobRequest{Path: file})
		if err != nil {
			t.Fatalf("job %d rejected: %v", i, err)
		}
		ids = append(ids, job.ID)
	}
	// Duplicate active request returns the existing job.
	dup, err := f.p.EnqueueImport(context.Background(), JobRequest{Path: filepath.Join(root, "Supernatural.S01E01.mkv")})
	if err != nil || dup.ID != ids[0] {
		t.Fatalf("duplicate enqueue: %v, %v", dup, err)
	}
	drain(t, f.p)
	for _, id := range ids {
		if j := mustJob(t, f.p, id); j.Status != JobImported || j.ImportedFiles != 1 {
			t.Fatalf("job %s: %+v", id, j)
		}
	}
	if n := len(f.svc.episodeFiles()); n != 3 {
		t.Fatalf("episode files registered = %d, want 3", n)
	}
}

func TestWorkersProcessQueueInBackground(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	dir, _ := writePack(t, t.TempDir(), 3)
	job, err := f.p.EnqueueImport(context.Background(), JobRequest{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	f.p.StartJobs()
	defer f.p.stopJobs()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if j := mustJob(t, f.p, job.ID); j.Status == JobImported {
			if j.ImportedFiles != 3 || j.ProcessedFiles != 3 || j.TotalFiles != 3 {
				t.Fatalf("progress counters wrong: %+v", j)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job never imported: %+v", mustJob(t, f.p, job.ID))
}

func TestPartialFailureRetriesOnlyFailedFiles(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	f.svc.fail["ep2"] = errors.New("database is locked")
	dir, files := writePack(t, t.TempDir(), 3)
	job, err := f.p.EnqueueImport(context.Background(), JobRequest{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	claimed, _ := f.p.claimJob(context.Background())
	f.p.runJob(context.Background(), claimed)

	j := mustJob(t, f.p, job.ID)
	if j.Status != JobRetryPending || j.ImportedFiles != 2 || j.FailedFiles != 1 || j.Error == "" || j.NextRetryAt == nil {
		t.Fatalf("partial failure must not report success: %+v", j)
	}
	// The scan reports the failed file as pending retry and the imported ones as imported.
	info, _ := os.Stat(files[1])
	if st, _, id := f.p.fileImportState(context.Background(), files[1]); st != string(JobRetryPending) || id != job.ID {
		t.Fatalf("failed file state = %q (%s)", st, id)
	}
	if _, ok := f.p.verifiedPriorImport(context.Background(), files[1], info); ok {
		t.Fatal("failed file reported as already imported")
	}
	info0, _ := os.Stat(files[0])
	if _, ok := f.p.verifiedPriorImport(context.Background(), files[0], info0); !ok {
		t.Fatal("imported file not verified")
	}

	delete(f.svc.fail, "ep2")
	time.Sleep(5 * time.Millisecond)
	drain(t, f.p)
	j = mustJob(t, f.p, job.ID)
	if j.Status != JobImported || j.ImportedFiles != 3 || j.FailedFiles != 0 {
		t.Fatalf("retry did not finish job: %+v", j)
	}
	for _, file := range j.Files {
		want := 1
		if file.SourcePath == files[1] {
			want = 2
		}
		if file.Attempts != want {
			t.Fatalf("%s attempted %d times, want %d", filepath.Base(file.SourcePath), file.Attempts, want)
		}
	}
}

func TestNonRetryableFailureFailsJobAndManualRetryRequeues(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	file := filepath.Join(t.TempDir(), "Unknown.Show.S01E01.mkv")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.svc.show.Title = "Something Else Entirely"
	job, err := f.p.EnqueueImport(context.Background(), JobRequest{Path: file})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, f.p)
	j := mustJob(t, f.p, job.ID)
	if j.Status != JobFailed || j.FailedFiles != 1 || j.Error == "" {
		t.Fatalf("unmatched file must fail explicitly: %+v", j)
	}
	if _, err := f.p.RetryJob(context.Background(), job.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if j := mustJob(t, f.p, job.ID); j.Status != JobQueued || j.Files[0].Status != FileQueued {
		t.Fatalf("retry did not requeue: %+v", j)
	}
	if _, err := f.p.RetryJob(context.Background(), job.ID); !errors.Is(err, ErrJobActive) {
		t.Fatalf("retrying active job: %v", err)
	}
	if _, err := f.p.RetryJob(context.Background(), "missing"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("retry unknown: %v", err)
	}
}

func TestRestartRecoversInterruptedJob(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	dir, _ := writePack(t, t.TempDir(), 2)
	job, err := f.p.EnqueueImport(context.Background(), JobRequest{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash mid-import.
	if _, err := f.p.claimJob(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.db.Exec(`UPDATE import_job_files SET status = 'importing' WHERE job_id = ?`, job.ID); err != nil {
		t.Fatal(err)
	}

	// A new pipeline over the same database resumes the job.
	opts := f.opts
	opts.JobWorkers = 1
	p2, err := NewPipeline(opts)
	if err != nil {
		t.Fatal(err)
	}
	p2.decisions = NewDecisionMaker()
	defer p2.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if j := mustJob(t, p2, job.ID); j.Status == JobImported && j.ImportedFiles == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job not recovered: %+v", mustJob(t, p2, job.ID))
}

func TestClientHeldFileWaitsWithoutSpendingAttempts(t *testing.T) {
	f := newPackPipeline(t, ImportModeMove, nil)
	dir, files := writePack(t, t.TempDir(), 2)
	client := &resolvePathFakeClient{id: "qbit", items: []downloads.Item{{
		ID: "h", Title: filepath.Base(dir), ContentPath: dir, Status: downloads.StatusItemSeeding, Progress: 1,
	}}}
	if err := f.opts.DownloadSvc.Registry().Register(client); err != nil {
		t.Fatal(err)
	}
	job, err := f.p.EnqueueImport(context.Background(), JobRequest{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		claimed, _ := f.p.claimJob(context.Background())
		if claimed == nil {
			// Waiting jobs are not due yet; force the next poll.
			if _, err := f.p.db.Exec(`UPDATE import_jobs SET next_retry_at = NULL WHERE id = ?`, job.ID); err != nil {
				t.Fatal(err)
			}
			claimed, _ = f.p.claimJob(context.Background())
		}
		f.p.runJob(context.Background(), claimed)
	}
	j := mustJob(t, f.p, job.ID)
	if j.Status != JobRetryPending || j.Attempts != 0 || j.Error == "" {
		t.Fatalf("held job should wait without spending attempts: %+v", j)
	}
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("held source touched: %v", err)
		}
	}
	// Torrent stopped: the next poll imports everything.
	client.items[0].Status = downloads.StatusItemPaused
	if _, err := f.p.db.Exec(`UPDATE import_jobs SET next_retry_at = NULL WHERE id = ?`, job.ID); err != nil {
		t.Fatal(err)
	}
	drain(t, f.p)
	if j := mustJob(t, f.p, job.ID); j.Status != JobImported || j.ImportedFiles != 2 {
		t.Fatalf("job did not resume after client released files: %+v", j)
	}
}

func TestScanMarksVerifiedImportsAndRejectsChangedFiles(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	_, files := writePack(t, t.TempDir(), 2)
	ev := &downloads.DownloadCompletedEvent{Title: "Supernatural.S01"}
	if _, err := f.p.importDownloadFile(context.Background(), ev, files[0], 2); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.importDownloadFile(context.Background(), ev, files[1], 2); err != nil {
		t.Fatal(err)
	}
	// Replace the second source with different content (same path).
	if err := os.WriteFile(files[1], []byte("a completely different and longer payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	r0 := f.p.scanSingleFile(context.Background(), files[0])
	if r0.SuggestedAction != "previously_imported" || r0.ImportStatus != "previously_imported" || r0.MatchedMediaID != "ep1" {
		t.Fatalf("verified import not reported: %+v", r0)
	}
	r1 := f.p.scanSingleFile(context.Background(), files[1])
	if r1.SuggestedAction == "already_imported" {
		t.Fatalf("changed file must not be reported as imported: %+v", r1)
	}
}

func TestImportJobHandlers(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	router := Router(f.p)
	root := t.TempDir()
	var paths []string
	for i := 1; i <= 3; i++ {
		file := filepath.Join(root, fmt.Sprintf("Supernatural.S01E%02d.mkv", i))
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, file)
	}
	paths = append(paths, filepath.Join(root, "missing.mkv"))

	do := func(method, target string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, target, &buf))
		return rec
	}

	rec := do(http.MethodPost, "/batch", map[string]any{"paths": paths})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("batch: %d %s", rec.Code, rec.Body)
	}
	var batch struct {
		Data   []ImportJob                    `json:"data"`
		Errors []struct{ Path, Error string } `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Data) != 3 || len(batch.Errors) != 1 || batch.Errors[0].Path != paths[3] {
		t.Fatalf("batch response: %s", rec.Body)
	}
	if rec := do(http.MethodPost, "/batch", map[string]any{"paths": []string{}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty batch: %d", rec.Code)
	}

	// Legacy manual endpoint queues instead of rejecting.
	extra := filepath.Join(root, "Supernatural.S01E04.mkv")
	_ = os.WriteFile(extra, []byte("x"), 0o600)
	rec = do(http.MethodPost, "/manual", map[string]string{"path": extra})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("manual: %d %s", rec.Code, rec.Body)
	}
	var manual struct {
		Status string    `json:"status"`
		JobID  string    `json:"job_id"`
		Data   ImportJob `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &manual)
	if manual.Status != "accepted" || manual.JobID == "" || manual.Data.ID != manual.JobID {
		t.Fatalf("manual response: %s", rec.Body)
	}

	rec = do(http.MethodGet, "/jobs", nil)
	var list struct {
		Data []ImportJob `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list.Data) != 4 {
		t.Fatalf("list jobs: %d %s", rec.Code, rec.Body)
	}

	if rec := do(http.MethodGet, "/jobs/"+manual.JobID, nil); rec.Code != http.StatusOK {
		t.Fatalf("get job: %d", rec.Code)
	}
	if rec := do(http.MethodGet, "/jobs/nope", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("get unknown job: %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/jobs/"+manual.JobID+"/retry", nil); rec.Code != http.StatusConflict {
		t.Fatalf("retry active job: %d", rec.Code)
	}
	drain(t, f.p)
	// E04 is not in the library, so it fails explicitly and can be retried.
	if j := mustJob(t, f.p, manual.JobID); j.Status != JobFailed || j.Files[0].Error == "" {
		t.Fatalf("unmatched manual import: %+v", j)
	}
	if rec := do(http.MethodPost, "/jobs/"+manual.JobID+"/retry", nil); rec.Code != http.StatusAccepted {
		t.Fatalf("retry failed job: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, "/jobs/"+batch.Data[0].ID+"/retry", nil); rec.Code != http.StatusConflict {
		t.Fatalf("retry imported job with nothing failed: %d %s", rec.Code, rec.Body)
	}
}

// insertHistory records a prior successful import; size 0 mimics a legacy
// row written before source fingerprints existed.
func insertHistory(t *testing.T, p *ImportPipeline, src, dest string, size, mtime int64, at time.Time) {
	t.Helper()
	if _, err := p.db.Exec(`INSERT INTO import_history
		(id, media_type, media_id, source_path, dest_path, import_mode, status, error, imported_at, source_size, source_mtime)
		VALUES (?, 'episode', 'ep1', ?, ?, 'copy', 'imported', '', ?, ?, ?)`,
		fmt.Sprintf("h-%d", time.Now().UnixNano()), src, dest, at, size, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestVerifiedPriorImportRequiresIdenticalContent(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "Supernatural.S01E01.mkv")
	dest := filepath.Join(root, "library-copy.mkv")
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(src, "original-bytes")
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(src, past, past); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(src)

	cases := []struct {
		name   string
		dest   string // destination contents; "" = missing
		size   int64
		mtime  int64
		wantOK bool
	}{
		{"legacy identical copy", "original-bytes", 0, 0, true},
		{"legacy same size different bytes", "corrupt-bytes!", 0, 0, false},
		{"legacy destination missing", "", 0, 0, false},
		{"fingerprint matches, identical copy", "original-bytes", info.Size(), info.ModTime().UnixNano(), true},
		{"fingerprint matches, destination replaced same size", "replaced-bytes", info.Size(), info.ModTime().UnixNano(), false},
		{"fingerprint matches, destination missing", "", info.Size(), info.ModTime().UnixNano(), false},
		{"stale fingerprint, identical copy", "original-bytes", info.Size(), info.ModTime().UnixNano() - 1, false},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.p.db.Exec(`DELETE FROM import_history`); err != nil {
				t.Fatal(err)
			}
			d := fmt.Sprintf("%s.%d", dest, i)
			if tc.dest != "" {
				write(d, tc.dest)
			}
			insertHistory(t, f.p, src, d, tc.size, tc.mtime, time.Now())
			if _, ok := f.p.verifiedPriorImport(ctx, src, info); ok != tc.wantOK {
				t.Fatalf("verifiedPriorImport = %v, want %v", ok, tc.wantOK)
			}
			r := f.p.scanSingleFile(ctx, src)
			if r.SuggestedAction == "already_imported" || r.SuggestedAction == "previously_imported" {
				t.Fatalf("legacy copies without destination metadata must not be hidden: %q", r.SuggestedAction)
			}
			got, err := f.p.completedImport(ctx, src)
			if err != nil {
				t.Fatal(err)
			}
			if (got != "") != (tc.dest != "" && !strings.Contains(tc.name, "different") && !strings.Contains(tc.name, "replaced")) {
				t.Fatalf("completedImport skip = %q", got)
			}
		})
	}
}

func TestLegacySameSizeDestinationIsNotSkippedOnImport(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	_, files := writePack(t, t.TempDir(), 1)
	dest := filepath.Join(f.lib, "Supernatural", "Season 01", "existing.mkv")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	// Same size as "episode 1" but different bytes.
	if err := os.WriteFile(dest, []byte("EPISODE 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	insertHistory(t, f.p, files[0], dest, 0, 0, time.Now())
	job, err := f.p.EnqueueImport(context.Background(), JobRequest{Path: files[0]})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, f.p)
	j := mustJob(t, f.p, job.ID)
	if j.Status != JobImported || j.Files[0].DestPath == dest {
		t.Fatalf("corrupt legacy destination certified the import: %+v", j.Files[0])
	}
	if got, _ := os.ReadFile(j.Files[0].DestPath); string(got) != "episode 1" {
		t.Fatalf("imported content = %q", got)
	}
}

func newSafetyPipeline(t *testing.T) (*packFixture, *safety.ReviewStore) {
	t.Helper()
	var store *safety.ReviewStore
	f := newPackPipeline(t, ImportModeCopy, func(o *PipelineOptions) {
		raw, err := os.ReadFile(filepath.Join("..", "storage", "migrations", "sqlite", "0021_manual_review.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := o.DB.Exec(strings.SplitN(string(raw), "-- +goose Down", 2)[0]); err != nil {
			t.Fatal(err)
		}
		cfg := safety.DefaultConfig()
		cfg.MinMovieSizeMB = 0
		o.PostVal = safety.NewPostValidator(cfg)
		store = safety.NewReviewStore(o.DB)
		o.ReviewStore = store
	})
	return f, store
}

func TestSafetyFlaggedJobIsHeldForReviewAndRetryCannotBypass(t *testing.T) {
	f, store := newSafetyPipeline(t)
	ctx := context.Background()
	dir, files := writePack(t, t.TempDir(), 2)
	if err := os.WriteFile(filepath.Join(dir, "setup.exe"), []byte("MZ"), 0o600); err != nil {
		t.Fatal(err)
	}
	job, err := f.p.EnqueueImport(ctx, JobRequest{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, f.p)
	j := mustJob(t, f.p, job.ID)
	if j.Status != JobPendingReview || j.PendingReviewFiles != 2 || j.ReviewID == "" || j.NextRetryAt != nil || j.Error == "" {
		t.Fatalf("flagged job not held for review: %+v", j)
	}
	if n := len(f.svc.episodeFiles()); n != 0 {
		t.Fatalf("flagged files imported: %d", n)
	}
	if st, _, _ := f.p.fileImportState(ctx, files[0]); st != string(FilePendingReview) {
		t.Fatalf("scan state = %q", st)
	}
	// Re-enqueueing returns the held job instead of bypassing it.
	again, err := f.p.EnqueueImport(ctx, JobRequest{Path: dir})
	if err != nil || again.ID != job.ID {
		t.Fatalf("re-enqueue bypassed review: %v, %v", again, err)
	}
	// Retry is refused while the review is pending, over HTTP too.
	if _, err := f.p.RetryJob(ctx, job.ID); !errors.Is(err, ErrPendingReview) {
		t.Fatalf("retry before approval: %v", err)
	}
	rec := httptest.NewRecorder()
	Router(f.p).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/jobs/"+job.ID+"/retry", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("retry endpoint: %d %s", rec.Code, rec.Body)
	}
	drain(t, f.p)
	if j := mustJob(t, f.p, job.ID); j.Status != JobPendingReview {
		t.Fatalf("held job was processed: %+v", j)
	}

	// Approving the review is the only way to release the job.
	pending, _ := store.ListPending(ctx)
	if len(pending) != 1 || pending[0].ID != j.ReviewID {
		t.Fatalf("reviews: %+v", pending)
	}
	if err := store.Approve(ctx, j.ReviewID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.RetryJob(ctx, job.ID); err != nil {
		t.Fatalf("retry after approval: %v", err)
	}
	drain(t, f.p)
	if j := mustJob(t, f.p, job.ID); j.Status != JobImported || j.ImportedFiles != 2 {
		t.Fatalf("approved job did not import: %+v", j)
	}
}

func TestRejectedReviewKeepsJobBlocked(t *testing.T) {
	f, store := newSafetyPipeline(t)
	ctx := context.Background()
	dir, _ := writePack(t, t.TempDir(), 1)
	if err := os.WriteFile(filepath.Join(dir, "run.bat"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	job, err := f.p.EnqueueImport(ctx, JobRequest{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, f.p)
	j := mustJob(t, f.p, job.ID)
	if err := store.Reject(ctx, j.ReviewID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.RetryJob(ctx, job.ID); !errors.Is(err, ErrPendingReview) {
		t.Fatalf("retry after rejection: %v", err)
	}
}
