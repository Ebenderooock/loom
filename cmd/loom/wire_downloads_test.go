package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/featureflags"
	"github.com/ebenderooock/loom/internal/imports"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/server"
	"github.com/ebenderooock/loom/internal/storage"
)

func TestWireDownloadsImportsWithAssignedQualityProfile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(ctx, config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "loom.db")},
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	profiles := qualityprofiles.NewStore(db.DB())
	if err := profiles.Create(ctx, &qualityprofiles.QualityProfile{
		ID: "profile", Name: "BluRay 1080p",
		Items: `[{"id":"bluray-1080p","name":"bluray-1080p","allowed":true}]`,
	}); err != nil {
		t.Fatal(err)
	}
	libraryRoot := t.TempDir()
	libStore := libraries.NewStore(db.DB())
	if err := libStore.Create(ctx, &libraries.Library{
		ID: "tv", Name: "TV", Path: libraryRoot, MediaType: "series",
	}); err != nil {
		t.Fatal(err)
	}
	seriesRepo := series.NewRepository(db.DB())
	if err := seriesRepo.CreateSeries(ctx, &series.Series{
		ID: "show", Title: "Supernatural", Year: 2005, LibraryID: "tv", QualityProfileID: "profile",
	}); err != nil {
		t.Fatal(err)
	}
	if err := seriesRepo.CreateSeason(ctx, &series.Season{ID: "season", SeriesID: "show", SeasonNumber: 15}); err != nil {
		t.Fatal(err)
	}
	if err := seriesRepo.CreateEpisode(ctx, &series.Episode{ID: "episode", SeriesID: "show", SeasonID: "season", EpisodeNumber: 1}); err != nil {
		t.Fatal(err)
	}
	downloadSvc, err := downloads.NewService(downloads.ServiceOptions{
		Repository: downloads.NewSQLiteRepository(db.DB()), Logger: logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	indexerSvc, err := indexers.NewService(indexers.ServiceOptions{
		Repository: indexers.NewSQLiteRepository(db.DB()), Logger: logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	moviesSvc := movies.NewService(movies.NewRepository(db.DB()))
	media := &mediaWiring{
		libStore: libStore, qpStore: profiles,
		seriesSvc: series.NewService(seriesRepo, ""),
	}
	wiring := &server.Wiring{
		Bus: eventbus.NewInProc(), FeatureFlags: featureflags.NewService(featureflags.NewStore(db.DB()), logger),
	}
	cfg := &config.Config{}
	cfg.MediaManagement.ImportMode = "copy"
	wired, err := wireDownloads(ctx, cfg, db, wiring, downloadSvc, moviesSvc, indexerSvc, media, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		wired.monitorCancel()
		wired.orchestratorCancel()
		wired.importPipeline.Stop()
	})
	for _, tc := range []struct {
		name       string
		resolution string
		reject     bool
	}{
		{name: "accepted by assigned profile", resolution: "1080p"},
		{name: "rejected by assigned profile", resolution: "720p", reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := "Supernatural (2005) - S15E01 - Back and to the Future (" + tc.resolution + " BluRay x265 Silence).mkv"
			source := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(source, []byte("episode media"), 0o600); err != nil {
				t.Fatal(err)
			}
			record, err := wired.importPipeline.ImportManualMatch(ctx, source, imports.MediaTypeEpisode, "episode")
			if tc.reject {
				if err == nil || !strings.Contains(err.Error(), "not allowed by profile") {
					t.Fatalf("want assigned profile rejection, got record=%+v err=%v", record, err)
				}
				if _, err := os.Stat(filepath.Join(libraryRoot, "Supernatural", "Season 15", name)); !os.IsNotExist(err) {
					t.Fatalf("disallowed episode was copied: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("allowed episode failed with production wiring: %v", err)
				}
				if got, err := os.ReadFile(record.DestPath); err != nil || string(got) != "episode media" {
					t.Fatalf("accepted episode missing or changed: %q, %v", got, err)
				}
				var files int
				if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_files WHERE episode_id = 'episode' AND file_path = ?`, record.DestPath).Scan(&files); err != nil || files != 1 {
					t.Fatalf("accepted episode not registered: count=%d err=%v", files, err)
				}
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("copy import consumed source: %v", err)
			}
		})
	}
}
