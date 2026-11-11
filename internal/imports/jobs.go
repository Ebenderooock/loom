package imports

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/mediafiles"
	"github.com/ebenderooock/loom/internal/safety"
)

// JobKind identifies how a queued import resolves its library match.
type JobKind string

// Job kinds: automatic matching, a user-chosen library item, or a reimport.
const (
	JobKindManual      JobKind = "manual"
	JobKindManualMatch JobKind = "manual_match"
	JobKindReimport    JobKind = "reimport"
)

// JobStatus is the lifecycle of a queued import job.
type JobStatus string

// Job statuses. Only imported and failed are terminal.
const (
	JobQueued        JobStatus = "queued"
	JobImporting     JobStatus = "importing"
	JobRetryPending  JobStatus = "retry_pending"
	JobImported      JobStatus = "imported"
	JobFailed        JobStatus = "failed"
	JobPendingReview JobStatus = "pending_review"
)

// FileStatus is the outcome of one media file inside a job.
type FileStatus string

// File statuses within a job.
const (
	FileQueued        FileStatus = "queued"
	FileImporting     FileStatus = "importing"
	FileImported      FileStatus = "imported"
	FileFailed        FileStatus = "failed"
	FileSkipped       FileStatus = "skipped"
	FilePendingReview FileStatus = "pending_review"
)

// ImportJob is a durable, user-visible import request for one path.
type ImportJob struct {
	ID                 string          `json:"id"`
	Kind               JobKind         `json:"kind"`
	SourcePath         string          `json:"source_path"`
	MediaType          string          `json:"media_type,omitempty"`
	MediaID            string          `json:"media_id,omitempty"`
	Status             JobStatus       `json:"status"`
	Attempts           int             `json:"attempts"`
	MaxAttempts        int             `json:"max_attempts"`
	TotalFiles         int             `json:"total_files"`
	ProcessedFiles     int             `json:"processed_files"`
	ImportedFiles      int             `json:"imported_files"`
	FailedFiles        int             `json:"failed_files"`
	SkippedFiles       int             `json:"skipped_files"`
	PendingReviewFiles int             `json:"pending_review_files"`
	Error              string          `json:"error,omitempty"`
	NextRetryAt        *time.Time      `json:"next_retry_at,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
	ReviewID           string          `json:"review_id,omitempty"`
	Files              []ImportJobFile `json:"files,omitempty"`

	conflictPolicy string
	workflowID     string
}

// ImportJobFile is the per-file outcome within an ImportJob.
type ImportJobFile struct {
	ID         string     `json:"id"`
	JobID      string     `json:"job_id"`
	SourcePath string     `json:"source_path"`
	DestPath   string     `json:"dest_path,omitempty"`
	MediaType  string     `json:"media_type,omitempty"`
	MediaID    string     `json:"media_id,omitempty"`
	Status     FileStatus `json:"status"`
	Attempts   int        `json:"attempts"`
	Error      string     `json:"error,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`

	retryable bool
}

// JobRequest describes a path to enqueue for import.
type JobRequest struct {
	Kind           JobKind
	Path           string
	MediaType      MediaType
	MediaID        string
	ConflictPolicy ConflictPolicy
}

var (
	// ErrJobNotFound is returned for an unknown job ID.
	ErrJobNotFound = errors.New("import job not found")
	// ErrJobActive is returned when retrying a job that is still running.
	ErrJobActive = errors.New("import job is still active")
	// ErrNothingToRetry is returned when a job has no failed files.
	ErrNothingToRetry = errors.New("import job has no failed files to retry")
	// ErrPendingReview is returned when retrying a job blocked by a safety
	// review that has not been approved.
	ErrPendingReview = errors.New("import job is awaiting safety review approval")
	// errWaitingForClient marks files a download client still owns.
	errWaitingForClient = errors.New("waiting for download client")
)

const (
	defaultJobWorkers     = 2
	defaultJobMaxAttempts = 5
	jobPollInterval       = 15 * time.Second
	clientWaitInterval    = 30 * time.Second
	finishedJobRetention  = 30 * 24 * time.Hour
)

var defaultJobBackoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

// jobQueue executes queued imports with a bounded number of workers. All
// state lives in the database so a restart resumes where it left off.
type jobQueue struct {
	workers int
	backoff []time.Duration
	poll    time.Duration
	now     func() time.Time

	wake chan struct{}
	stop chan struct{}
	wg   sync.WaitGroup

	mu      sync.Mutex
	started bool
	stopped bool

	fileLocks sync.Map // cleaned source path -> *sync.Mutex
}

func newJobQueue(workers int) *jobQueue {
	if workers <= 0 {
		workers = defaultJobWorkers
	}
	return &jobQueue{
		workers: workers,
		backoff: defaultJobBackoff,
		poll:    jobPollInterval,
		now:     func() time.Time { return time.Now().UTC() },
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
	}
}

func (q *jobQueue) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *jobQueue) lockFile(path string) func() {
	v, _ := q.fileLocks.LoadOrStore(filepath.Clean(path), &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

// StartJobs recovers interrupted jobs and starts the bounded worker pool.
// It is safe to call more than once.
func (p *ImportPipeline) StartJobs() {
	q := p.jobs
	q.mu.Lock()
	if q.started || q.stopped {
		q.mu.Unlock()
		return
	}
	q.started = true
	q.mu.Unlock()

	ctx := context.Background()
	if err := p.recoverJobs(ctx); err != nil {
		p.logger.Error("recover import jobs", "error", err)
	}
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go p.jobWorker()
	}
	q.signal()
}

// stopJobs stops the worker pool and waits for in-flight files to finish.
func (p *ImportPipeline) stopJobs() {
	q := p.jobs
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return
	}
	q.stopped = true
	q.mu.Unlock()
	close(q.stop)
	q.wg.Wait()
}

// recoverJobs re-queues work interrupted by a shutdown or crash and prunes
// old finished jobs.
func (p *ImportPipeline) recoverJobs(ctx context.Context) error {
	now := p.jobs.now()
	if _, err := p.db.ExecContext(ctx, `UPDATE import_job_files SET status = ?, updated_at = ? WHERE status = ?`,
		string(FileQueued), now, string(FileImporting)); err != nil {
		return err
	}
	if _, err := p.db.ExecContext(ctx, `UPDATE import_jobs SET status = ?, updated_at = ? WHERE status = ?`,
		string(JobQueued), now, string(JobImporting)); err != nil {
		return err
	}
	cutoff := now.Add(-finishedJobRetention)
	if _, err := p.db.ExecContext(ctx, `DELETE FROM import_job_files WHERE job_id IN (
		SELECT id FROM import_jobs WHERE status IN (?, ?) AND completed_at IS NOT NULL AND completed_at < ?)`,
		string(JobImported), string(JobFailed), cutoff); err != nil {
		return err
	}
	_, err := p.db.ExecContext(ctx, `DELETE FROM import_jobs WHERE status IN (?, ?) AND completed_at IS NOT NULL AND completed_at < ?`,
		string(JobImported), string(JobFailed), cutoff)
	return err
}

func (p *ImportPipeline) jobWorker() {
	q := p.jobs
	defer q.wg.Done()
	ticker := time.NewTicker(q.poll)
	defer ticker.Stop()
	for {
		select {
		case <-q.stop:
			return
		default:
		}
		job, err := p.claimJob(context.Background())
		if err != nil {
			p.logger.Error("claim import job", "error", err)
		}
		if job != nil {
			p.runJob(context.Background(), job)
			continue
		}
		select {
		case <-q.stop:
			return
		case <-q.wake:
		case <-ticker.C:
		}
	}
}

// EnqueueImport validates a path, records a durable job with one row per
// media file and wakes the workers. An identical active (or review-held)
// request returns the existing job instead of creating a duplicate.
func (p *ImportPipeline) EnqueueImport(ctx context.Context, req JobRequest) (*ImportJob, error) {
	if req.Kind == "" {
		req.Kind = JobKindManual
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	path = filepath.Clean(path)
	switch req.Kind {
	case JobKindManual:
	case JobKindManualMatch, JobKindReimport:
		if req.MediaType == "" || req.MediaID == "" {
			return nil, fmt.Errorf("media_type and media_id are required")
		}
		switch req.MediaType {
		case MediaTypeMovie, MediaTypeEpisode:
		case MediaTypeMusic:
			// Music matches link exactly one audio file to one track.
			if err := checkRegularSource(path); err != nil {
				return nil, err
			}
			if !mediafiles.IsAudio(strings.ToLower(filepath.Ext(path))) {
				return nil, fmt.Errorf("not an audio file: %s", path)
			}
		default:
			return nil, fmt.Errorf("unsupported media type: %s", req.MediaType)
		}
	default:
		return nil, fmt.Errorf("unknown import kind: %s", req.Kind)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing symlink import path: %s", path)
	}
	if req.Kind == JobKindReimport && info.IsDir() {
		return nil, fmt.Errorf("source path is a directory, expected a file")
	}
	var files []string
	if info.IsDir() {
		if files, err = scanImportFiles(ctx, path); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
	} else {
		ext := strings.ToLower(filepath.Ext(path))
		if !mediaExtensions[ext] && !mediafiles.IsAudio(ext) {
			return nil, fmt.Errorf("not a media file: %s", path)
		}
		files = []string{path}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no media files found in %s", path)
	}

	var existing string
	err = p.db.QueryRowContext(ctx, `SELECT id FROM import_jobs
		WHERE source_path = ? AND kind = ? AND media_type = ? AND media_id = ? AND status IN (?, ?, ?, ?)
		ORDER BY created_at DESC LIMIT 1`,
		path, string(req.Kind), string(req.MediaType), req.MediaID,
		string(JobQueued), string(JobImporting), string(JobRetryPending), string(JobPendingReview)).Scan(&existing)
	if err == nil {
		return p.GetJob(ctx, existing, false)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check existing import job: %w", err)
	}

	var wfID string
	if p.wfEngine != nil {
		var ids []string
		if req.MediaID != "" {
			ids = []string{req.MediaID}
		}
		if wf, wfErr := p.wfEngine.StartImport(ctx, string(req.MediaType), ids, filepath.Base(path)); wfErr != nil {
			p.logger.Warn("failed to create import workflow", "path", path, "error", wfErr)
		} else {
			wfID = wf.ID
		}
	}

	now := p.jobs.now()
	id := uuid.New().String()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO import_jobs
		(id, kind, source_path, media_type, media_id, conflict_policy, workflow_id, status, attempts, max_attempts, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, '', ?, ?)`,
		id, string(req.Kind), path, string(req.MediaType), req.MediaID, string(req.ConflictPolicy), wfID,
		string(JobQueued), defaultJobMaxAttempts, now, now); err != nil {
		return nil, fmt.Errorf("create import job: %w", err)
	}
	for _, f := range files {
		if _, err := tx.ExecContext(ctx, `INSERT INTO import_job_files (id, job_id, source_path, status, updated_at)
			VALUES (?, ?, ?, ?, ?)`, uuid.New().String(), id, f, string(FileQueued), now); err != nil {
			return nil, fmt.Errorf("create import job file: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	p.logger.Info("import job queued", "job_id", id, "kind", req.Kind, "path", path, "files", len(files))
	p.jobs.signal()
	return p.GetJob(ctx, id, false)
}

// RetryJob re-queues the failed files of a finished or waiting job. Files
// that already imported are left untouched.
func (p *ImportPipeline) RetryJob(ctx context.Context, id string) (*ImportJob, error) {
	job, err := p.GetJob(ctx, id, false)
	if err != nil {
		return nil, err
	}
	if job.Status == JobQueued || job.Status == JobImporting {
		return nil, ErrJobActive
	}
	// The safety gate is never bypassed by a retry: files held for review
	// only become retryable once the review has been approved.
	resettable := []any{string(FileFailed), string(FileQueued)}
	if job.Status == JobPendingReview || job.PendingReviewFiles > 0 {
		if !p.reviewApproved(ctx, job.ReviewID) {
			return nil, ErrPendingReview
		}
		resettable = append(resettable, string(FilePendingReview))
	}
	now := p.jobs.now()
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(resettable)), ", ")
	args := append([]any{string(FileQueued), now, id}, resettable...)
	// #nosec G202 -- placeholders is a constant-shaped list of "?" markers.
	res, err := p.db.ExecContext(ctx, `UPDATE import_job_files SET status = ?, error = '', retryable = 1, attempts = 0, updated_at = ?
		WHERE job_id = ? AND status IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("reset failed import files: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNothingToRetry
	}
	wfID := job.workflowID
	if (job.Status == JobFailed || job.Status == JobPendingReview) && p.wfEngine != nil {
		var ids []string
		if job.MediaID != "" {
			ids = []string{job.MediaID}
		}
		if wf, wfErr := p.wfEngine.StartImport(ctx, job.MediaType, ids, filepath.Base(job.SourcePath)); wfErr == nil {
			wfID = wf.ID
		}
	}
	if _, err := p.db.ExecContext(ctx, `UPDATE import_jobs SET status = ?, attempts = 0, error = '', next_retry_at = NULL,
		completed_at = NULL, workflow_id = ?, updated_at = ? WHERE id = ?`,
		string(JobQueued), wfID, now, id); err != nil {
		return nil, fmt.Errorf("requeue import job: %w", err)
	}
	p.jobs.signal()
	return p.GetJob(ctx, id, false)
}

// claimJob atomically moves the oldest runnable job into importing.
func (p *ImportPipeline) claimJob(ctx context.Context) (*ImportJob, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, status, next_retry_at FROM import_jobs
		WHERE status IN (?, ?) ORDER BY created_at ASC, id ASC`,
		string(JobQueued), string(JobRetryPending))
	if err != nil {
		return nil, err
	}
	type cand struct {
		id, status string
	}
	var cands []cand
	now := p.jobs.now()
	for rows.Next() {
		var c cand
		var next sql.NullTime
		if err := rows.Scan(&c.id, &c.status, &next); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if c.status == string(JobRetryPending) && next.Valid && next.Time.After(now) {
			continue
		}
		cands = append(cands, c)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, c := range cands {
		res, err := p.db.ExecContext(ctx, `UPDATE import_jobs SET status = ?, attempts = attempts + 1, next_retry_at = NULL, updated_at = ?
			WHERE id = ? AND status = ?`, string(JobImporting), now, c.id, c.status)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			return p.GetJob(ctx, c.id, true)
		}
	}
	return nil, nil
}

// runJob processes the pending files of a claimed job and settles its state.
func (p *ImportPipeline) runJob(ctx context.Context, job *ImportJob) {
	p.logger.Info("import job starting", "job_id", job.ID, "path", job.SourcePath, "attempt", job.Attempts)

	var pending []ImportJobFile
	primary := 0
	for _, f := range job.Files {
		if !strings.Contains(strings.ToLower(filepath.Base(f.SourcePath)), "sample") {
			primary++
		}
		switch f.Status {
		case FileQueued, FileImporting:
			pending = append(pending, f)
		case FileFailed:
			if p.fileRetryDue(job, f) {
				pending = append(pending, f)
			}
		}
	}

	var match *MatchResult
	var matchErr error
	if job.Kind == JobKindManualMatch && MediaType(job.MediaType) != MediaTypeMusic {
		match, matchErr = p.matcher.MatchExact(ctx, MediaType(job.MediaType), job.MediaID)
		if matchErr != nil {
			matchErr = fmt.Errorf("resolve media: %w", matchErr)
		}
	}

	if len(pending) > 0 && matchErr == nil {
		if blocked := p.safetyGate(ctx, job, pending); blocked {
			p.settleJob(ctx, job.ID, 0)
			return
		}
	}

	holds := p.newClientHolds()
	waiting := 0
	for _, f := range pending {
		select {
		case <-p.jobs.stop:
			// Shutting down: leave remaining files queued for recovery.
			p.settleJob(ctx, job.ID, 0)
			return
		default:
		}
		if holder := holds.holder(ctx, f.SourcePath); holder != "" {
			waiting++
			p.setFileState(ctx, f.ID, FileQueued, "", "", "", false, true,
				fmt.Sprintf("%v: %s is still active in the download client", errWaitingForClient, holder))
			continue
		}
		p.setFileState(ctx, f.ID, FileImporting, "", "", "", false, true, "")
		var dest string
		var resMatch *MatchResult
		var err error
		if matchErr != nil {
			err = matchErr
		} else {
			dest, resMatch, err = p.safeImportJobFile(ctx, job, f.SourcePath, primary, match)
		}
		mt, mid := "", ""
		if resMatch != nil {
			mt, mid = string(resMatch.MediaType), resMatch.MediaID
		}
		switch {
		case err == nil:
			p.setFileState(ctx, f.ID, FileImported, dest, mt, mid, true, true, "")
		case errors.Is(err, errImportSkipped):
			p.setFileState(ctx, f.ID, FileSkipped, "", mt, mid, true, false, err.Error())
		case isClientHoldError(err):
			waiting++
			p.setFileState(ctx, f.ID, FileQueued, "", mt, mid, false, true,
				fmt.Sprintf("%v: %v", errWaitingForClient, err))
		default:
			p.setFileState(ctx, f.ID, FileFailed, "", mt, mid, true, isRetryableImportError(err), err.Error())
		}
	}
	p.settleJob(ctx, job.ID, waiting)
}

// safeImportJobFile converts a panic while importing one file into a file
// failure so a single bad file cannot take down the worker pool.
func (p *ImportPipeline) safeImportJobFile(ctx context.Context, job *ImportJob, file string, primary int, match *MatchResult) (dest string, m *MatchResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("import job file panicked", "job_id", job.ID, "file", file, "panic", r)
			err = fmt.Errorf("internal import error: %v", r)
		}
	}()
	return p.importJobFile(ctx, job, file, primary, match)
}

// importJobFile imports one file of a job according to its kind.
func (p *ImportPipeline) importJobFile(ctx context.Context, job *ImportJob, file string, primary int, match *MatchResult) (string, *MatchResult, error) {
	unlock := p.jobs.lockFile(file)
	defer unlock()
	if _, err := os.Lstat(file); err != nil {
		return "", nil, fmt.Errorf("source file missing: %w", err)
	}
	ev := &downloads.DownloadCompletedEvent{Title: filepath.Base(job.SourcePath), CompletedAt: time.Now()}
	switch job.Kind {
	case JobKindManualMatch:
		if MediaType(job.MediaType) == MediaTypeMusic {
			rec, err := p.importMusicMatch(ctx, file, job.MediaID)
			if err != nil {
				return "", nil, err
			}
			return rec.DestPath, &MatchResult{MediaType: MediaTypeMusic, MediaID: rec.MediaID}, nil
		}
		if dest, err := p.completedImport(ctx, file); err != nil || dest != "" {
			return dest, match, err
		}
		dest, err := p.importMatchedWith(ctx, ev, match, file, filepath.Base(file), true)
		return dest, match, err
	case JobKindReimport:
		policy := ConflictPolicy(job.conflictPolicy)
		rec, err := p.ReimportFile(ctx, MediaType(job.MediaType), job.MediaID, file, ReimportOptions{ConflictPolicy: policy})
		if err != nil {
			return "", nil, err
		}
		m := &MatchResult{MediaType: rec.MediaType, MediaID: rec.MediaID}
		if rec.Status != StatusImported {
			return "", m, fmt.Errorf("%w: %s", errImportSkipped, rec.Error)
		}
		return rec.DestPath, m, nil
	default:
		dest, err := p.importDownloadFile(ctx, ev, file, primary)
		if err != nil {
			return "", nil, err
		}
		m := p.historyMatch(ctx, file, dest)
		return dest, m, nil
	}
}

// historyMatch recovers the media linkage recorded for a completed import.
func (p *ImportPipeline) historyMatch(ctx context.Context, source, dest string) *MatchResult {
	var mt, mid string
	if err := p.db.QueryRowContext(ctx, `SELECT media_type, media_id FROM import_history
		WHERE source_path = ? AND dest_path = ? AND status = ? ORDER BY imported_at DESC LIMIT 1`,
		source, dest, string(StatusImported)).Scan(&mt, &mid); err != nil {
		return nil
	}
	return &MatchResult{MediaType: MediaType(mt), MediaID: mid}
}

func (p *ImportPipeline) setFileState(ctx context.Context, id string, status FileStatus, dest, mediaType, mediaID string, countAttempt, retryable bool, errMsg string) {
	inc := 0
	if countAttempt {
		inc = 1
	}
	retry := 0
	if retryable {
		retry = 1
	}
	if _, err := p.db.ExecContext(ctx, `UPDATE import_job_files SET status = ?,
		dest_path = CASE WHEN ? = '' THEN dest_path ELSE ? END,
		media_type = CASE WHEN ? = '' THEN media_type ELSE ? END,
		media_id = CASE WHEN ? = '' THEN media_id ELSE ? END,
		attempts = attempts + ?, retryable = ?, error = ?, updated_at = ? WHERE id = ?`,
		string(status), dest, dest, mediaType, mediaType, mediaID, mediaID, inc, retry, errMsg, p.jobs.now(), id); err != nil {
		p.logger.Error("update import job file", "id", id, "error", err)
	}
	// Touch the job so pollers see progress.
	if _, err := p.db.ExecContext(ctx, `UPDATE import_jobs SET updated_at = ? WHERE id = (SELECT job_id FROM import_job_files WHERE id = ?)`,
		p.jobs.now(), id); err != nil {
		p.logger.Error("touch import job", "id", id, "error", err)
	}
}

// settleJob derives the job state from its files. A job only reports
// imported when every non-skipped file imported; anything else is retried
// with backoff or failed explicitly.
func (p *ImportPipeline) settleJob(ctx context.Context, id string, waiting int) {
	job, err := p.GetJob(ctx, id, true)
	if err != nil {
		p.logger.Error("load import job for settlement", "id", id, "error", err)
		return
	}
	now := p.jobs.now()
	queued, retryableFailed := 0, 0
	var firstErr string
	var nextDue time.Time
	for _, f := range job.Files {
		switch f.Status {
		case FileQueued, FileImporting:
			queued++
		case FileFailed:
			if f.retryable && f.Attempts < job.MaxAttempts {
				retryableFailed++
				due := f.UpdatedAt.Add(p.jobBackoff(f.Attempts))
				if nextDue.IsZero() || due.Before(nextDue) {
					nextDue = due
				}
			}
			if firstErr == "" {
				firstErr = fmt.Sprintf("%s: %s", filepath.Base(f.SourcePath), f.Error)
			}
		}
	}
	if waiting > 0 && firstErr == "" {
		for _, f := range job.Files {
			if f.Status == FileQueued && f.Error != "" {
				firstErr = f.Error
				break
			}
		}
	}

	status := JobImported
	var next *time.Time
	attempts := job.Attempts
	var errMsg string
	switch {
	case job.PendingReviewFiles > 0 && queued == 0 && waiting == 0:
		// Held by the safety gate: never retried automatically.
		status = JobPendingReview
		attempts = max(job.Attempts-1, 0)
		for _, f := range job.Files {
			if f.Status == FilePendingReview {
				errMsg = f.Error
				break
			}
		}
	case waiting > 0:
		// A download client still owns some files: keep polling without
		// spending a retry attempt. Failed siblings keep their own per-file
		// backoff (see fileRetryDue).
		status = JobRetryPending
		t := now.Add(clientWaitInterval)
		next = &t
		attempts = max(job.Attempts-1, 0)
		errMsg = firstErr
	case queued > 0:
		// Interrupted (shutdown); resume promptly.
		status = JobQueued
		attempts = max(job.Attempts-1, 0)
	case job.FailedFiles > 0:
		errMsg = fmt.Sprintf("%d of %d files failed; %s", job.FailedFiles, job.TotalFiles, firstErr)
		if retryableFailed > 0 {
			status = JobRetryPending
			t := nextDue
			if t.Before(now) {
				t = now
			}
			next = &t
		} else {
			status = JobFailed
		}
	case job.ImportedFiles == 0:
		status = JobFailed
		errMsg = "no files imported (all files were skipped)"
		for _, f := range job.Files {
			if f.Status == FileSkipped && f.Error != "" {
				errMsg += ": " + f.Error
				break
			}
		}
	}

	var completed any
	if status == JobImported || status == JobFailed {
		completed = now
	}
	var nextArg any
	if next != nil {
		nextArg = *next
	}
	if _, err := p.db.ExecContext(ctx, `UPDATE import_jobs SET status = ?, attempts = ?, error = ?, next_retry_at = ?,
		completed_at = ?, updated_at = ? WHERE id = ?`,
		string(status), attempts, errMsg, nextArg, completed, now, id); err != nil {
		p.logger.Error("settle import job", "id", id, "error", err)
		return
	}
	p.logger.Info("import job settled", "job_id", id, "status", status,
		"imported", job.ImportedFiles, "failed", job.FailedFiles, "skipped", job.SkippedFiles, "waiting", waiting)
	if p.wfEngine != nil && job.workflowID != "" {
		switch status {
		case JobImported:
			if err := p.wfEngine.CompleteImport(ctx, job.workflowID); err != nil {
				p.logger.Warn("complete import workflow", "id", job.workflowID, "error", err)
			}
		case JobFailed, JobPendingReview:
			if err := p.wfEngine.FailImport(ctx, job.workflowID, errMsg); err != nil {
				p.logger.Warn("fail import workflow", "id", job.workflowID, "error", err)
			}
		}
	}
	if status == JobRetryPending || status == JobQueued {
		p.jobs.signal()
	}
}

// safetyGate runs post-download validation for manual imports, mirroring
// the automatic pipeline. A flagged source creates (or reuses) a manual
// review and marks the pending files pending_review; it is only lifted by
// approving that review. Reimports of library files are not gated.
func (p *ImportPipeline) safetyGate(ctx context.Context, job *ImportJob, pending []ImportJobFile) bool {
	if p.postVal == nil || job.Kind == JobKindReimport {
		return false
	}
	if job.ReviewID != "" && p.reviewApproved(ctx, job.ReviewID) {
		return false
	}
	result, err := p.postVal.ValidateDownload(job.SourcePath)
	if err == nil && result.Pass {
		return false
	}
	reason := strings.Join(result.Reasons, "; ")
	if err != nil && reason == "" {
		reason = err.Error()
	}
	p.logger.Warn("manual import flagged by post-validator", "job_id", job.ID, "path", job.SourcePath, "reasons", reason)
	reviewID := job.ReviewID
	if p.reviewStore != nil && (reviewID == "" || p.reviewRejected(ctx, reviewID)) {
		mediaType, mediaID := job.MediaType, job.MediaID
		if mediaID == "" {
			mediaType, mediaID = "import_job", job.ID
		}
		if r, rErr := p.reviewStore.Create(ctx, mediaType, mediaID, job.SourcePath, reason); rErr != nil {
			p.logger.Error("failed to create review entry", "job_id", job.ID, "error", rErr)
		} else {
			reviewID = r.ID
		}
	}
	if _, uErr := p.db.ExecContext(ctx, `UPDATE import_jobs SET review_id = ? WHERE id = ?`, reviewID, job.ID); uErr != nil {
		p.logger.Error("store import job review", "job_id", job.ID, "error", uErr)
	}
	_ = p.recordStatus(ctx, job.MediaType, job.MediaID, job.SourcePath, "", StatusPendingReview, reason)
	for _, f := range pending {
		p.setFileState(ctx, f.ID, FilePendingReview, "", "", "", false, false, "pending safety review: "+reason)
	}
	return true
}

func (p *ImportPipeline) reviewStatus(ctx context.Context, id string) string {
	if id == "" {
		return ""
	}
	var status string
	if err := p.db.QueryRowContext(ctx, `SELECT status FROM manual_review WHERE id = ?`, id).Scan(&status); err != nil {
		return ""
	}
	return status
}

func (p *ImportPipeline) reviewApproved(ctx context.Context, id string) bool {
	return p.reviewStatus(ctx, id) == string(safety.ReviewApproved)
}

func (p *ImportPipeline) reviewRejected(ctx context.Context, id string) bool {
	return p.reviewStatus(ctx, id) == string(safety.ReviewRejected)
}

// fileRetryDue reports whether a failed file may be retried automatically:
// it must be retryable, under the attempt cap and past its backoff.
func (p *ImportPipeline) fileRetryDue(job *ImportJob, f ImportJobFile) bool {
	if !f.retryable || f.Attempts >= job.MaxAttempts {
		return false
	}
	return !p.jobs.now().Before(f.UpdatedAt.Add(p.jobBackoff(f.Attempts)))
}

func (p *ImportPipeline) jobBackoff(attempt int) time.Duration {
	b := p.jobs.backoff
	if len(b) == 0 {
		return time.Minute
	}
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(b) {
		return b[len(b)-1]
	}
	return b[attempt-1]
}

// isRetryableImportError reports whether retrying the same file could
// succeed without user intervention.
func isRetryableImportError(err error) bool {
	if errors.Is(err, ErrNoMatch) || errors.Is(err, ErrDestinationExists) {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"import rejected", "not a media file", "refusing symlink", "source file missing", "unsupported media type",
		"not an audio file", "does not improve existing track", "not in the album's selected edition"} {
		if strings.Contains(msg, s) {
			return false
		}
	}
	return true
}

// isClientHoldError recognises filesystem errors caused by another process
// (typically a download client) holding the file.
func isClientHoldError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"file in use", "being used by another process", "resource busy",
		"sharing violation", "text file busy", "blocked by download client"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// clientHolds lazily snapshots download client items to detect files that
// a client is still writing (or seeding, for move imports).
type clientHolds struct {
	p      *ImportPipeline
	loaded bool
	items  []heldItem
}

type heldItem struct {
	path  string
	title string
}

func (p *ImportPipeline) newClientHolds() *clientHolds { return &clientHolds{p: p} }

func (h *clientHolds) load(ctx context.Context) {
	h.loaded = true
	if h.p.downloadSvc == nil || h.p.downloadSvc.Registry() == nil || h.p.downloadSvc.Registry().Len() == 0 {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	agg := h.p.downloadSvc.Registry().Status(cctx, nil, downloads.FanOutOptions{PerClientTimeout: 15 * time.Second})
	for _, it := range agg.Items {
		if !h.p.itemHoldsFiles(it) {
			continue
		}
		var paths []string
		if it.ContentPath != "" {
			paths = append(paths, it.ContentPath)
		}
		if it.SavePath != "" && it.Title != "" {
			paths = append(paths, filepath.Join(it.SavePath, it.Title))
		}
		for _, raw := range paths {
			mapped := h.p.applyRemotePathMapping(cctx, it.ClientID, raw)
			if mapped == "" || isUnresolvedContentPath(mapped) {
				continue
			}
			h.items = append(h.items, heldItem{path: filepath.Clean(mapped), title: it.Title})
		}
	}
}

// itemHoldsFiles reports whether a client item may still change or need
// its files: actively downloading, or seeding when the import would move
// (consume) the source.
func (p *ImportPipeline) itemHoldsFiles(it downloads.Item) bool {
	switch it.Status {
	case downloads.StatusItemQueued, downloads.StatusItemDownloading:
		return true
	case downloads.StatusItemSeeding:
		return p.importMode == ImportModeMove
	}
	return it.Progress > 0 && it.Progress < 1 && it.Status != downloads.StatusItemPaused &&
		it.Status != downloads.StatusItemFailed
}

func (h *clientHolds) holder(ctx context.Context, file string) string {
	if !h.loaded {
		h.load(ctx)
	}
	file = filepath.Clean(file)
	for _, it := range h.items {
		if file == it.path || strings.HasPrefix(file, it.path+string(filepath.Separator)) {
			if it.title != "" {
				return fmt.Sprintf("%q", it.title)
			}
			return fmt.Sprintf("%q", it.path)
		}
	}
	return ""
}

const jobColumns = `j.id, j.kind, j.source_path, j.media_type, j.media_id, j.conflict_policy, j.workflow_id, j.review_id, j.status,
	j.attempts, j.max_attempts, j.error, j.next_retry_at, j.created_at, j.updated_at, j.completed_at,
	COUNT(f.id),
	COALESCE(SUM(CASE WHEN f.status = 'imported' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN f.status = 'failed' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN f.status = 'skipped' THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN f.status = 'pending_review' THEN 1 ELSE 0 END), 0)`

type rowScanner interface{ Scan(...any) error }

func scanJob(r rowScanner) (*ImportJob, error) {
	var j ImportJob
	var next, completed sql.NullTime
	if err := r.Scan(&j.ID, &j.Kind, &j.SourcePath, &j.MediaType, &j.MediaID, &j.conflictPolicy, &j.workflowID, &j.ReviewID, &j.Status,
		&j.Attempts, &j.MaxAttempts, &j.Error, &next, &j.CreatedAt, &j.UpdatedAt, &completed,
		&j.TotalFiles, &j.ImportedFiles, &j.FailedFiles, &j.SkippedFiles, &j.PendingReviewFiles); err != nil {
		return nil, err
	}
	j.ProcessedFiles = j.ImportedFiles + j.FailedFiles + j.SkippedFiles + j.PendingReviewFiles
	if next.Valid {
		t := next.Time
		j.NextRetryAt = &t
	}
	if completed.Valid {
		t := completed.Time
		j.CompletedAt = &t
	}
	return &j, nil
}

// GetJob returns a job, optionally with its per-file outcomes.
func (p *ImportPipeline) GetJob(ctx context.Context, id string, withFiles bool) (*ImportJob, error) {
	row := p.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM import_jobs j
		LEFT JOIN import_job_files f ON f.job_id = j.id WHERE j.id = ? GROUP BY j.id`, id)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get import job: %w", err)
	}
	if withFiles {
		if job.Files, err = p.jobFiles(ctx, id); err != nil {
			return nil, err
		}
	}
	return job, nil
}

func (p *ImportPipeline) jobFiles(ctx context.Context, jobID string) ([]ImportJobFile, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, job_id, source_path, dest_path, media_type, media_id, status, attempts, retryable, error, updated_at
		FROM import_job_files WHERE job_id = ? ORDER BY source_path`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list import job files: %w", err)
	}
	defer func() { _ = rows.Close() }()
	files := []ImportJobFile{}
	for rows.Next() {
		var f ImportJobFile
		var retry int
		if err := rows.Scan(&f.ID, &f.JobID, &f.SourcePath, &f.DestPath, &f.MediaType, &f.MediaID, &f.Status,
			&f.Attempts, &retry, &f.Error, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.retryable = retry != 0
		files = append(files, f)
	}
	return files, rows.Err()
}

// ListJobs returns every active job plus up to limit most recent finished
// jobs, newest first.
func (p *ImportPipeline) ListJobs(ctx context.Context, limit int) ([]*ImportJob, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	active := []any{string(JobQueued), string(JobImporting), string(JobRetryPending)}
	query := func(where string, args ...any) ([]*ImportJob, error) {
		// #nosec G202 -- jobColumns and where are constant SQL; values are bound args.
		rows, err := p.db.QueryContext(ctx, `SELECT `+jobColumns+` FROM import_jobs j
			LEFT JOIN import_job_files f ON f.job_id = j.id `+where, args...)
		if err != nil {
			return nil, fmt.Errorf("list import jobs: %w", err)
		}
		defer func() { _ = rows.Close() }()
		var out []*ImportJob
		for rows.Next() {
			j, err := scanJob(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, j)
		}
		return out, rows.Err()
	}
	jobs, err := query(`WHERE j.status IN (?, ?, ?) GROUP BY j.id`, active...)
	if err != nil {
		return nil, err
	}
	finished, err := query(`WHERE j.status NOT IN (?, ?, ?) GROUP BY j.id ORDER BY j.created_at DESC LIMIT ?`,
		append(active, limit)...)
	if err != nil {
		return nil, err
	}
	jobs = append(jobs, finished...)
	sort.SliceStable(jobs, func(a, b int) bool { return jobs[a].CreatedAt.After(jobs[b].CreatedAt) })
	if jobs == nil {
		jobs = []*ImportJob{}
	}
	return jobs, nil
}

// fileImportState reports the most recent queue outcome for a source file.
func (p *ImportPipeline) fileImportState(ctx context.Context, path string) (status, errMsg, jobID string) {
	var fs, js string
	err := p.db.QueryRowContext(ctx, `SELECT f.status, f.error, f.job_id, j.status FROM import_job_files f
		JOIN import_jobs j ON j.id = f.job_id WHERE f.source_path = ? ORDER BY f.updated_at DESC LIMIT 1`, path).
		Scan(&fs, &errMsg, &jobID, &js)
	if err != nil {
		return "", "", ""
	}
	status = fs
	if FileStatus(fs) == FileQueued && JobStatus(js) == JobRetryPending {
		status = string(JobRetryPending)
	}
	if FileStatus(fs) == FileFailed && JobStatus(js) == JobRetryPending {
		status = string(JobRetryPending)
	}
	if FileStatus(fs) == FileSkipped || FileStatus(fs) == FileImported {
		// Imported is only reported after verifiedPriorImport succeeds.
		status = ""
	}
	return status, errMsg, jobID
}

// verifiedPriorImport reports whether import history proves this exact
// source file was imported: the recorded destination must still exist and
// hold identical bytes (same inode or matching SHA-256). A recorded source
// fingerprint that no longer matches rejects cheaply without hashing; it is
// never sufficient on its own to certify the destination.
func (p *ImportPipeline) verifiedPriorImport(ctx context.Context, path string, info os.FileInfo) (*ImportRecord, bool) {
	if info == nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, false
	}
	var dest, mediaType, mediaID string
	var size, mtime int64
	var importedAt time.Time
	err := p.db.QueryRowContext(ctx, `SELECT dest_path, media_type, media_id, COALESCE(source_size, 0), COALESCE(source_mtime, 0), imported_at
		FROM import_history WHERE source_path = ? AND status = ? AND dest_path != ''
		ORDER BY imported_at DESC LIMIT 1`, path, string(StatusImported)).Scan(&dest, &mediaType, &mediaID, &size, &mtime, &importedAt)
	if err != nil {
		return nil, false
	}
	if size > 0 && (size != info.Size() || mtime != info.ModTime().UnixNano()) {
		return nil, false
	}
	destInfo, err := os.Lstat(dest)
	if err != nil {
		return nil, false
	}
	same, err := p.sameContent(path, dest, info, destInfo)
	if err != nil || !same {
		return nil, false
	}
	return &ImportRecord{SourcePath: path, DestPath: dest, MediaType: MediaType(mediaType), MediaID: mediaID,
		Status: StatusImported, ImportedAt: importedAt}, true
}
