package main

import (
	"context"
	"log/slog"

	"github.com/ebenderooock/loom/internal/compat/prowlarrv1"
	"github.com/ebenderooock/loom/internal/compat/radarrv3"
	"github.com/ebenderooock/loom/internal/compat/sonarrv3"
	"github.com/ebenderooock/loom/internal/compat/syncprofiles"
	"github.com/ebenderooock/loom/internal/connect"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/healthmonitor"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/notifications"
	"github.com/ebenderooock/loom/internal/scheduler"
	"github.com/ebenderooock/loom/internal/server"
	"github.com/ebenderooock/loom/internal/storage"
)

// infraWiring holds lifecycle objects produced by wireInfra
// so the caller can manage their shutdown.
type infraWiring struct {
	notifDispatcher *notifications.Dispatcher
	rollingSearcher *scheduler.RollingSearcher
	healthMon       *healthmonitor.Monitor
}

// wireInfra constructs infrastructure services (connect, compat shims,
// notification dispatcher, rolling search, health monitor) and mounts
// them on srv.
func wireInfra(
	ctx context.Context,
	db storage.DB,
	srv *server.Server,
	indexerSvc *indexers.Service,
	downloadSvc *downloads.Service,
	moviesSvc movies.Service,
	media *mediaWiring,
	logger *slog.Logger,
) (*infraWiring, error) {
	// Connect (media server integrations)
	connectSvc := connect.NewService(db.DB())
	srv.SetConnect(connectSvc)

	// Notification dispatcher — subscribes to domain events on the bus
	// and fans out to all matching notification connections.
	notifDispatcher := notifications.NewDispatcher(srv.Bus(), media.notifSvc, logger)

	// *arr API compatibility shims
	syncStore := syncprofiles.NewStore(db.DB())
	srv.SetSyncProfileStore(syncStore)
	srv.SetCompatRadarr(radarrv3.NewHandler(moviesSvc, media.libStore, media.qpStore, logger))
	srv.SetCompatSonarr(sonarrv3.NewHandler(media.seriesSvc, media.libStore, media.qpStore, logger))
	srv.SetCompatProwlarr(prowlarrv1.NewHandler(indexerSvc, syncStore, logger))

	// Rolling-search scheduler
	rsCfg := scheduler.DefaultRollingSearchConfig()
	rsStore := scheduler.NewStore(db.DB())
	rollingSearcher := scheduler.NewRollingSearcher(rsStore, indexerSvc, logger, rsCfg)
	srv.SetRollingSearch(rollingSearcher)

	// Health monitor
	healthMon := buildHealthMonitor(ctx, indexerSvc, downloadSvc, media.notifSvc, media.libStore, logger)
	srv.SetHealthMonitor(healthMon)

	return &infraWiring{
		notifDispatcher: notifDispatcher,
		rollingSearcher: rollingSearcher,
		healthMon:       healthMon,
	}, nil
}

// --- Health monitor wiring ---

// indexerHealthAdapter wraps *indexers.Service to satisfy healthmonitor.IndexerChecker.
type indexerHealthAdapter struct {
	svc *indexers.Service
}

func (a *indexerHealthAdapter) List(ctx context.Context) ([]healthmonitor.IndexerInfo, error) {
	defs, err := a.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]healthmonitor.IndexerInfo, len(defs))
	for i, d := range defs {
		status := "unknown"
		if d.Health != nil {
			status = string(d.Health.Status)
		}
		out[i] = healthmonitor.IndexerInfo{
			ID: d.ID, Name: d.Name, Enabled: d.Enabled, Status: status,
		}
	}
	return out, nil
}

// downloadHealthAdapter wraps *downloads.Service to satisfy healthmonitor.DownloadChecker.
type downloadHealthAdapter struct {
	svc *downloads.Service
}

func (a *downloadHealthAdapter) ListClients(ctx context.Context) ([]healthmonitor.ClientInfo, error) {
	defs, err := a.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]healthmonitor.ClientInfo, len(defs))
	for i, d := range defs {
		status := "unknown"
		if d.Health != nil {
			status = string(d.Health.Status)
		}
		out[i] = healthmonitor.ClientInfo{
			ID: d.ID, Name: d.Name, Enabled: d.Enabled, Status: status,
		}
	}
	return out, nil
}

func buildHealthMonitor(
	ctx context.Context,
	indexerSvc *indexers.Service,
	downloadSvc *downloads.Service,
	notifSvc notifications.Service,
	libStore *libraries.Store,
	logger *slog.Logger,
) *healthmonitor.Monitor {
	// Collect library paths for disk-space checks.
	var libPaths []string
	if libStore != nil {
		libs, err := libStore.List(ctx)
		if err == nil {
			for _, l := range libs {
				if l.Path != "" {
					libPaths = append(libPaths, l.Path)
				}
			}
		}
	}

	// Build notification sender closure.
	var notifier healthmonitor.NotificationSender
	if notifSvc != nil {
		notifier = func(ctx context.Context, title, body string) error {
			return notifSvc.Send(ctx, notifications.EventOnHealthIssue, title, body, nil)
		}
	}

	var idxChecker healthmonitor.IndexerChecker
	if indexerSvc != nil {
		idxChecker = &indexerHealthAdapter{svc: indexerSvc}
	}
	var dlChecker healthmonitor.DownloadChecker
	if downloadSvc != nil {
		dlChecker = &downloadHealthAdapter{svc: downloadSvc}
	}

	return healthmonitor.New(healthmonitor.Options{
		Indexers:  idxChecker,
		Downloads: dlChecker,
		Notifier:  notifier,
		LibPaths:  libPaths,
		Logger:    logger.With("component", "health-monitor"),
	})
}
