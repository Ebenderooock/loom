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
	"testing"
	"time"
)

func dismissRequest(t *testing.T, p *ImportPipeline, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	Router(p).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/jobs/dismiss", &buf))
	return rec
}

func TestDismissInactiveJobsPreservesActiveWorkFilesAndHistory(t *testing.T) {
	ctx := context.Background()
	f := newPackPipeline(t, ImportModeCopy, nil)
	root := t.TempDir()
	statuses := []JobStatus{JobImported, JobFailed, JobPendingReview, JobQueued, JobImporting, JobRetryPending}
	ids := make([]string, 0, len(statuses))
	for i, status := range statuses {
		path := filepath.Join(root, fmt.Sprintf("Supernatural.S01E%02d.mkv", i+1))
		if err := os.WriteFile(path, []byte("retained source"), 0o600); err != nil {
			t.Fatal(err)
		}
		job, err := f.p.EnqueueImport(ctx, JobRequest{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, job.ID)
		if _, err := f.p.db.Exec(`UPDATE import_jobs SET status = ? WHERE id = ?`, string(status), job.ID); err != nil {
			t.Fatal(err)
		}
	}
	dest := filepath.Join(f.lib, "retained.mkv")
	if err := os.WriteFile(dest, []byte("retained destination"), 0o600); err != nil {
		t.Fatal(err)
	}
	insertHistory(t, f.p, mustJob(t, f.p, ids[0]).SourcePath, dest, 15, 0, time.Now())
	rec := dismissRequest(t, f.p, map[string]any{"all_inactive": true})
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"dismissed":3}` {
		t.Fatalf("dismiss inactive: %d %s", rec.Code, rec.Body)
	}
	listed, err := f.p.ListJobs(ctx, 100)
	if err != nil || len(listed) != 3 {
		t.Fatalf("list after dismissal: %+v, %v", listed, err)
	}
	for _, job := range listed {
		if job.Status != JobQueued && job.Status != JobImporting && job.Status != JobRetryPending {
			t.Fatalf("inactive job still visible: %+v", job)
		}
	}
	for i, id := range ids {
		job := mustJob(t, f.p, id)
		if job.Status != statuses[i] || len(job.Files) != 1 {
			t.Fatalf("dismissal altered job history: %+v", job)
		}
		if b, err := os.ReadFile(job.SourcePath); err != nil || string(b) != "retained source" {
			t.Fatalf("dismissal altered source: %q, %v", b, err)
		}
	}
	if b, err := os.ReadFile(dest); err != nil || string(b) != "retained destination" {
		t.Fatalf("dismissal altered destination: %q, %v", b, err)
	}
	var history int
	if err := f.p.db.QueryRow(`SELECT COUNT(*) FROM import_history`).Scan(&history); err != nil || history != 1 {
		t.Fatalf("dismissal altered import history: count=%d, error=%v", history, err)
	}
}

func TestSelectedDismissalIsAtomicIdempotentAndRestoredByRetry(t *testing.T) {
	ctx := context.Background()
	f := newPackPipeline(t, ImportModeCopy, nil)
	_, paths := writePack(t, t.TempDir(), 2)
	failed, err := f.p.EnqueueImport(ctx, JobRequest{Path: paths[0]})
	if err != nil {
		t.Fatal(err)
	}
	active, err := f.p.EnqueueImport(ctx, JobRequest{Path: paths[1]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.db.Exec(`UPDATE import_jobs SET status = 'failed' WHERE id = ?`, failed.ID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		body any
		code int
	}{
		{"empty", map[string]any{}, http.StatusBadRequest},
		{"ambiguous", map[string]any{"ids": []string{failed.ID}, "all_inactive": true}, http.StatusBadRequest},
		{"blank ID", map[string]any{"ids": []string{""}}, http.StatusBadRequest},
		{"active", map[string]any{"ids": []string{failed.ID, active.ID}}, http.StatusConflict},
		{"unknown", map[string]any{"ids": []string{failed.ID, "missing"}}, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := dismissRequest(t, f.p, tc.body)
			if rec.Code != tc.code {
				t.Fatalf("dismissal: %d %s", rec.Code, rec.Body)
			}
			listed, err := f.p.ListJobs(ctx, 100)
			if err != nil || len(listed) != 2 {
				t.Fatalf("rejected dismissal changed queue: %+v, %v", listed, err)
			}
		})
	}
	for i, expected := range []int{1, 0} {
		rec := dismissRequest(t, f.p, map[string]any{"ids": []string{failed.ID, failed.ID}})
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != fmt.Sprintf(`{"dismissed":%d}`, expected) {
			t.Fatalf("dismissal %d: %d %s", i, rec.Code, rec.Body)
		}
	}
	if _, err := f.p.RetryJob(ctx, failed.ID); err != nil {
		t.Fatal(err)
	}
	listed, err := f.p.ListJobs(ctx, 100)
	if err != nil || len(listed) != 2 || mustJob(t, f.p, failed.ID).Status != JobQueued {
		t.Fatalf("explicit retry did not restore queue entry: %+v, %v", listed, err)
	}
}

func TestDismissedReviewHeldJobCanBeRequeuedWithoutBypassingReview(t *testing.T) {
	ctx := context.Background()
	f := newPackPipeline(t, ImportModeCopy, nil)
	_, paths := writePack(t, t.TempDir(), 1)
	job, err := f.p.EnqueueImport(ctx, JobRequest{Path: paths[0]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.db.Exec(`UPDATE import_jobs SET status = 'pending_review' WHERE id = ?`, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.DismissJobs(ctx, []string{job.ID}, false); err != nil {
		t.Fatal(err)
	}
	restored, err := f.p.EnqueueImport(ctx, JobRequest{Path: paths[0]})
	if err != nil || restored.ID != job.ID || restored.Status != JobPendingReview {
		t.Fatalf("requeue must restore original review hold: %+v, %v", restored, err)
	}
	listed, err := f.p.ListJobs(ctx, 100)
	if err != nil || len(listed) != 1 {
		t.Fatalf("review-held entry not restored: %+v, %v", listed, err)
	}
	if _, err := f.p.RetryJob(ctx, job.ID); !errors.Is(err, ErrPendingReview) {
		t.Fatalf("dismissal bypassed review approval: %v", err)
	}
}

func TestClearInactiveJobsIsNotLimitedToVisibleQueuePage(t *testing.T) {
	ctx := context.Background()
	f := newPackPipeline(t, ImportModeCopy, nil)
	now := time.Now().UTC()
	for i := 0; i < 105; i++ {
		if _, err := f.p.db.Exec(`INSERT INTO import_jobs
			(id, kind, source_path, status, created_at, updated_at)
			VALUES (?, 'manual', '/deleted/download.mkv', 'failed', ?, ?)`,
			fmt.Sprintf("old-%d", i), now, now); err != nil {
			t.Fatal(err)
		}
	}
	if jobs, err := f.p.ListJobs(ctx, 100); err != nil || len(jobs) != 100 {
		t.Fatalf("queue limit fixture: %d jobs, %v", len(jobs), err)
	}
	if n, err := f.p.DismissJobs(ctx, nil, true); err != nil || n != 105 {
		t.Fatalf("clear inactive: %d, %v", n, err)
	}
	if jobs, err := f.p.ListJobs(ctx, 100); err != nil || len(jobs) != 0 {
		t.Fatalf("old entries reappeared: %d jobs, %v", len(jobs), err)
	}
}
