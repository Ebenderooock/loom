package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
	"github.com/ebenderooock/loom/internal/cleanup"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/imports"
	"github.com/ebenderooock/loom/internal/kernel/scheduler"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/storage"
)

// cleanupJobName is the scheduler key for the periodic downloads-cleanup sweep.
const cleanupJobName = "downloads-cleanup"

// cleanupSchedule runs the sweep every 6 hours.
const cleanupSchedule = "0 */6 * * *"

// buildCleanupService wires the downloads-cleanup service. Its scan roots come
// from each enabled download client's effective save folder; its tracked-path
// set unions active downloads and recent import sources; and deletion routes
// through the import recycle bin when configured.
func buildCleanupService(
	db storage.DB,
	dlSvc *downloads.Service,
	pipeline *imports.ImportPipeline,
	libStore *libraries.Store,
	appCfg *appconfig.Config,
	logger *slog.Logger,
) *cleanup.Service {
	store := cleanup.NewStore(db.DB())
	rb := imports.NewRecycleBin(appCfg)

	roots := func(ctx context.Context) ([]cleanup.Root, error) {
		defs, err := dlSvc.List(ctx)
		if err != nil {
			return nil, err
		}
		seen := make(map[string]bool)
		var out []cleanup.Root
		for _, d := range defs {
			if !d.Enabled {
				continue
			}
			root := d.SavePathDefault
			// The built-in torrent engine knows its effective download dir,
			// which may differ from the persisted save_path_default.
			if c, ok := dlSvc.Registry().Get(d.ID); ok {
				if tm, ok := c.(downloads.TorrentManager); ok {
					if sp := tm.EngineSummary().SavePath; sp != "" {
						root = sp
					}
				}
			}
			if root == "" || seen[root] {
				continue
			}
			seen[root] = true
			out = append(out, cleanup.Root{Path: root, ClientID: d.ID, ClientName: d.Name})
		}
		return out, nil
	}

	tracked := func(ctx context.Context) ([]string, error) {
		// Active downloads are mandatory: without them we cannot safely
		// classify orphans. If any client's status query failed the set is
		// incomplete, so abort the scan/delete rather than risk treating a
		// live download from an unreachable client as an orphan.
		active, complete := dlSvc.TrackedDownloadPaths(ctx)
		if !complete {
			return nil, fmt.Errorf("download client status incomplete; skipping cleanup to avoid deleting live downloads")
		}
		paths := append([]string(nil), active...)
		// Recent import source paths protect files mid-import or just imported.
		if pipeline != nil {
			if recs, err := pipeline.ListHistory(ctx, 1000, 0); err == nil {
				for _, r := range recs {
					if r != nil && r.SourcePath != "" {
						paths = append(paths, r.SourcePath)
					}
				}
			} else {
				logger.Warn("cleanup: import history lookup failed", "err", err)
			}
		}
		return paths, nil
	}

	// protected resolves media library roots. Any download root or orphan that
	// overlaps one of these is never scanned or deleted. A lookup error aborts
	// the operation so library content can never be misclassified.
	protected := func(ctx context.Context) ([]string, error) {
		if libStore == nil {
			return nil, nil
		}
		libs, err := libStore.List(ctx)
		if err != nil {
			return nil, err
		}
		var paths []string
		for _, l := range libs {
			if l.Path != "" {
				paths = append(paths, l.Path)
			}
		}
		return paths, nil
	}

	recycle := func(_ context.Context, path, _ string) error {
		return cleanupRemove(rb, path)
	}

	return cleanup.NewService(cleanup.Options{
		Store:     store,
		Roots:     roots,
		Tracked:   tracked,
		Protected: protected,
		Recycle:   recycle,
		Logger:    logger,
	})
}

// cleanupRemove recycles an orphan entry (file or directory) into the recycle
// bin when one is configured, otherwise deletes it. It never silently falls
// back to deletion when recycling is requested but fails — the error surfaces
// so the user can investigate.
func cleanupRemove(rb *imports.RecycleBin, path string) error {
	if rb != nil && rb.Enabled && rb.Path != "" {
		dest := filepath.Join(rb.Path, "downloads-cleanup",
			fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(path)))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.Rename(path, dest)
	}
	return os.RemoveAll(path)
}

// registerCleanupJob hooks the periodic scan + auto-delete sweep into the
// scheduler.
func registerCleanupJob(ctx context.Context, sched *scheduler.Scheduler, svc *cleanup.Service) error {
	handler := func(ctx context.Context) error {
		if _, err := svc.Scan(ctx); err != nil {
			return err
		}
		if _, err := svc.AutoDelete(ctx); err != nil {
			return err
		}
		return nil
	}
	return sched.Register(ctx, cleanupJobName, cleanupSchedule, handler, []byte(`{"builtin":true}`))
}
