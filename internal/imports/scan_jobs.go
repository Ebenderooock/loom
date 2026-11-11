package imports

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	scanRetention = 10 * time.Minute
	maxScanJobs   = 8
	maxScanActive = 2
	maxScanPage   = 1000
)

var (
	errScansBusy    = errors.New("two folder scans are already running; wait for one to finish")
	errScansStopped = errors.New("folder scanning is stopping")
)

// ImportScan is an ephemeral, read-only scan with cursor-paged results.
type ImportScan struct {
	ID             string       `json:"id"`
	Path           string       `json:"path"`
	Status         string       `json:"status"`
	Phase          string       `json:"phase"`
	TotalFiles     int          `json:"total_files"`
	ProcessedFiles int          `json:"processed_files"`
	CurrentFile    string       `json:"current_file,omitempty"`
	Error          string       `json:"error,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	CompletedAt    *time.Time   `json:"completed_at,omitempty"`
	Results        []ScanResult `json:"results"`
	NextOffset     int          `json:"next_offset"`
}

type scanTask struct {
	scan     ImportScan
	cancel   context.CancelFunc
	finished bool
}

type scanQueue struct {
	mu      sync.RWMutex
	tasks   map[string]*scanTask
	wg      sync.WaitGroup
	stopped bool
}

// StartFolderScan returns before touching the filesystem or matching files.
// Slow mounts and content verification must never hold an HTTP request open.
func (p *ImportPipeline) StartFolderScan(ctx context.Context, path string) (*ImportScan, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("path is required")
	}
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("resolve scan path: %w", err)
	}
	q := p.scans
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return nil, errScansStopped
	}
	now := time.Now().UTC()
	active := 0
	for id, task := range q.tasks {
		if !task.finished {
			active++
			if task.scan.Path == path && task.scan.Status == "running" {
				return scanPage(task, 0, 0), nil
			}
		} else if now.Sub(task.scan.UpdatedAt) > scanRetention {
			delete(q.tasks, id)
		}
	}
	if active >= maxScanActive {
		return nil, errScansBusy
	}
	for len(q.tasks) >= maxScanJobs {
		var oldest *scanTask
		for _, task := range q.tasks {
			if task.finished && (oldest == nil || task.scan.UpdatedAt.Before(oldest.scan.UpdatedAt)) {
				oldest = task
			}
		}
		if oldest == nil {
			return nil, errScansBusy
		}
		delete(q.tasks, oldest.scan.ID)
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	task := &scanTask{
		scan: ImportScan{
			ID: uuid.NewString(), Path: path, Status: "running", Phase: "discovering",
			CreatedAt: now, UpdatedAt: now, Results: []ScanResult{},
		},
		cancel: cancel,
	}
	q.tasks[task.scan.ID] = task
	q.wg.Add(1)
	go p.runFolderScan(ctx, task)
	return scanPage(task, 0, 0), nil
}

func scanPage(task *scanTask, offset, limit int) *ImportScan {
	scan := task.scan
	offset = max(0, min(offset, len(scan.Results)))
	limit = max(0, min(limit, maxScanPage))
	end := min(offset+limit, len(scan.Results))
	scan.Results = append([]ScanResult{}, scan.Results[offset:end]...)
	scan.NextOffset = end
	return &scan
}

// GetFolderScan returns a bounded snapshot without performing filesystem work.
func (p *ImportPipeline) GetFolderScan(id string, offset, limit int) *ImportScan {
	q := p.scans
	q.mu.RLock()
	defer q.mu.RUnlock()
	task := q.tasks[id]
	if task == nil || (task.finished && time.Since(task.scan.UpdatedAt) > scanRetention) {
		return nil
	}
	return scanPage(task, offset, limit)
}

// CancelFolderScan cancels verification while retaining any completed results.
func (p *ImportPipeline) CancelFolderScan(id string) *ImportScan {
	q := p.scans
	q.mu.Lock()
	defer q.mu.Unlock()
	task := q.tasks[id]
	if task == nil || (task.finished && time.Since(task.scan.UpdatedAt) > scanRetention) {
		return nil
	}
	if !task.finished && task.scan.Status == "running" {
		task.cancel()
		now := time.Now().UTC()
		task.scan.Status = "cancelled"
		task.scan.CurrentFile = ""
		task.scan.UpdatedAt = now
		task.scan.CompletedAt = &now
	}
	return scanPage(task, 0, 0)
}

func (p *ImportPipeline) runFolderScan(ctx context.Context, task *scanTask) {
	defer p.scans.wg.Done()
	defer task.cancel()
	p.logger.Info("folder scan started", "scan_id", task.scan.ID, "path", task.scan.Path)
	files, err := discoverImportFiles(ctx, task.scan.Path, func(path string) {
		p.scans.mu.Lock()
		task.scan.TotalFiles++
		task.scan.CurrentFile = path
		task.scan.UpdatedAt = time.Now().UTC()
		p.scans.mu.Unlock()
	})
	if err == nil {
		p.scans.mu.Lock()
		task.scan.Phase = "matching"
		p.scans.mu.Unlock()
		for _, file := range files {
			if err = ctx.Err(); err != nil {
				break
			}
			p.scans.mu.Lock()
			task.scan.CurrentFile = file
			task.scan.UpdatedAt = time.Now().UTC()
			p.scans.mu.Unlock()
			result := p.scanSingleFile(ctx, file)
			p.scans.mu.Lock()
			err = ctx.Err()
			if err == nil {
				task.scan.Results = append(task.scan.Results, result)
				task.scan.ProcessedFiles++
				task.scan.UpdatedAt = time.Now().UTC()
			}
			p.scans.mu.Unlock()
			if err != nil {
				break
			}
		}
	}
	p.scans.mu.Lock()
	now := time.Now().UTC()
	task.finished = true
	task.scan.CurrentFile = ""
	task.scan.UpdatedAt = now
	task.scan.CompletedAt = &now
	switch {
	case ctx.Err() != nil:
		task.scan.Status = "cancelled"
	case err != nil:
		task.scan.Status = "failed"
		task.scan.Error = err.Error()
	default:
		task.scan.Status = "completed"
	}
	status := task.scan.Status
	p.scans.mu.Unlock()
	if err != nil && ctx.Err() == nil {
		p.logger.Error("folder scan failed", "scan_id", task.scan.ID, "error", err)
	} else {
		p.logger.Info("folder scan finished", "scan_id", task.scan.ID, "status", status)
	}
}

func (p *ImportPipeline) stopScans() {
	q := p.scans
	q.mu.Lock()
	q.stopped = true
	for _, task := range q.tasks {
		if !task.finished {
			task.cancel()
		}
	}
	q.mu.Unlock()
	q.wg.Wait()
}
