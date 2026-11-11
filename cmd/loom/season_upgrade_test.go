package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestWireDownloadsFullSeasonReplacementRetry(t *testing.T) {
	const (
		episodes            = 23
		failedEpisode       = 12
		oldSize       int64 = 400 << 20
		newSize       int64 = 1100 << 20
	)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	root := t.TempDir() // DB, downloads, and library share a filesystem for sparse hardlinks.
	db, err := storage.Open(ctx, config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(root, "loom.db")},
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
	libraryRoot := filepath.Join(root, "library")
	libStore := libraries.NewStore(db.DB())
	if err := libStore.Create(ctx, &libraries.Library{
		ID: "tv", Name: "TV", Path: libraryRoot, MediaType: "series",
	}); err != nil {
		t.Fatal(err)
	}
	repo := series.NewRepository(db.DB())
	svc := series.NewService(repo, "")
	if err := repo.CreateSeries(ctx, &series.Series{
		ID: "show", Title: "Supernatural", Year: 2005, LibraryID: "tv", QualityProfileID: "profile",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateSeason(ctx, &series.Season{
		ID: "season", SeriesID: "show", SeasonNumber: 10, EpisodeCount: episodes,
	}); err != nil {
		t.Fatal(err)
	}
	seasonFolder := filepath.Join(root, "downloads", "Supernatural (2005) Season 10")
	type episodeFixture struct {
		id, fileID, source, dest, oldMarker, newMarker string
		oldInfo, newInfo                               os.FileInfo
	}
	fixtures := make([]episodeFixture, episodes)
	created := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	for i := range fixtures {
		n := i + 1
		f := &fixtures[i]
		f.id, f.fileID = fmt.Sprintf("episode-%02d", n), fmt.Sprintf("original-file-%02d", n)
		f.source = filepath.Join(seasonFolder, fmt.Sprintf("Supernatural (2005) - S10E%02d - Episode %02d (1080p BluRay x265 Silence).mkv", n, n))
		// Canonical episode destination selected by production buildDestFilename.
		f.dest = filepath.Join(libraryRoot, "Supernatural", "Season 10", fmt.Sprintf("Supernatural - S10E%02d.mkv", n))
		f.oldMarker, f.newMarker = fmt.Sprintf("old-episode-%02d", n), fmt.Sprintf("new-episode-%02d", n)
		f.oldInfo = writeSeasonSparseFile(t, f.dest, oldSize, f.oldMarker)
		f.newInfo = writeSeasonSparseFile(t, f.source, newSize, f.newMarker)
		if err := repo.CreateEpisode(ctx, &series.Episode{
			ID: f.id, SeriesID: "show", SeasonID: "season", EpisodeNumber: n, Title: fmt.Sprintf("Episode %02d", n),
		}); err != nil {
			t.Fatal(err)
		}
		if err := svc.CreateEpisodeFile(ctx, &series.EpisodeFile{
			ID: f.fileID, EpisodeID: f.id, SeriesID: "show", FilePath: f.dest,
			FileSize: oldSize, Quality: "bluray-1080p", CreatedAt: created, UpdatedAt: created,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Fail precisely one registration, after replacement has published the new
	// inode. The importer must restore the old file and retain the new source.
	if _, err := db.DB().ExecContext(ctx, `CREATE TRIGGER fail_season_episode_registration
		BEFORE UPDATE ON episode_files WHEN OLD.episode_id = 'episode-12'
		BEGIN SELECT RAISE(FAIL, 'forced season episode registration failure'); END`); err != nil {
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
	media := &mediaWiring{libStore: libStore, qpStore: profiles, seriesSvc: svc}
	wiring := &server.Wiring{
		Bus: eventbus.NewInProc(), FeatureFlags: featureflags.NewService(featureflags.NewStore(db.DB()), logger),
	}
	// Leave ImportMode empty to exercise production's default move behavior.
	wired, err := wireDownloads(ctx, &config.Config{}, db, wiring, downloadSvc,
		movies.NewService(movies.NewRepository(db.DB())), indexerSvc, media, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		wired.monitorCancel()
		wired.orchestratorCancel()
		wired.importPipeline.Stop()
	})
	job, err := wired.importPipeline.EnqueueImport(ctx, imports.JobRequest{Path: seasonFolder})
	if err != nil {
		t.Fatal(err)
	}
	wait := func(phase string, done func(*imports.ImportJob) bool) *imports.ImportJob {
		t.Helper()
		deadline := time.NewTimer(15 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		var last *imports.ImportJob
		for {
			last, err = wired.importPipeline.GetJob(ctx, job.ID, true)
			if err != nil {
				t.Fatalf("%s GetJob: %v; last=%+v", phase, err, last)
			}
			if done(last) {
				return last
			}
			if last.Status == imports.JobPendingReview || last.Status == imports.JobImported || last.Status == imports.JobFailed {
				t.Fatalf("%s unexpected terminal status: %+v; files=%+v", phase, last, last.Files)
			}
			select {
			case <-ctx.Done():
				t.Fatalf("%s context: %v; job=%+v; files=%+v", phase, ctx.Err(), last, last.Files)
			case <-deadline.C:
				t.Fatalf("%s timeout: job=%+v; files=%+v", phase, last, last.Files)
			case <-ticker.C:
			}
		}
	}
	first := wait("first attempt", func(j *imports.ImportJob) bool {
		return j.Status == imports.JobRetryPending || j.Status == imports.JobFailed
	})
	if first.TotalFiles != episodes || len(first.Files) != episodes || first.ProcessedFiles != episodes ||
		first.ImportedFiles != episodes-1 || first.FailedFiles != 1 || first.SkippedFiles != 0 ||
		first.PendingReviewFiles != 0 || first.ReviewID != "" {
		t.Fatalf("first attempt outcomes: %+v; files=%+v", first, first.Files)
	}
	assertRecord := func(f episodeFixture, size int64, quality string) {
		t.Helper()
		var count, linked int
		if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_files WHERE episode_id = ?`, f.id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s file records=%d err=%v", f.id, count, err)
		}
		var id, seriesID, path, gotQuality, gotCreated string
		var gotSize int64
		if err := db.DB().QueryRowContext(ctx, `SELECT id, series_id, file_path, file_size, quality, created_at
			FROM episode_files WHERE episode_id = ?`, f.id).Scan(&id, &seriesID, &path, &gotSize, &gotQuality, &gotCreated); err != nil {
			t.Fatal(err)
		}
		if id != f.fileID || seriesID != "show" || path != f.dest || gotSize != size || gotQuality != quality || gotCreated != created.Format(time.RFC3339) {
			t.Fatalf("%s registration: id=%s series=%s path=%s size=%d quality=%s created=%s",
				f.id, id, seriesID, path, gotSize, gotQuality, gotCreated)
		}
		if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM episodes WHERE id = ? AND has_file = 1`, f.id).Scan(&linked); err != nil || linked != 1 {
			t.Fatalf("%s episode linkage=%d err=%v", f.id, linked, err)
		}
	}
	firstFiles := make(map[string]imports.ImportJobFile, episodes)
	for _, file := range first.Files {
		if _, exists := firstFiles[file.SourcePath]; exists {
			t.Fatalf("duplicate file outcome: %+v", file)
		}
		firstFiles[file.SourcePath] = file
	}
	for i, f := range fixtures {
		file, ok := firstFiles[f.source]
		if !ok || file.Attempts != 1 {
			t.Fatalf("%s missing or unexpected attempts: %+v", f.id, file)
		}
		if i+1 == failedEpisode {
			if file.Status != imports.FileFailed || !strings.Contains(file.Error, "forced season episode registration failure") {
				t.Fatalf("injected failure not observed: %+v", file)
			}
			assertSeasonSparseFile(t, f.source, newSize, f.newMarker, f.newInfo)
			assertSeasonSparseFile(t, f.dest, oldSize, f.oldMarker, f.oldInfo)
			assertRecord(f, oldSize, "bluray-1080p")
		} else {
			if file.Status != imports.FileImported || file.MediaID != f.id || file.MediaType != string(imports.MediaTypeEpisode) || file.DestPath != f.dest {
				t.Fatalf("%s wrong success outcome: %+v", f.id, file)
			}
			if _, err := os.Lstat(f.source); !os.IsNotExist(err) {
				t.Fatalf("%s successful move source remains: %v", f.id, err)
			}
			assertSeasonSparseFile(t, f.dest, newSize, f.newMarker, f.newInfo)
			assertRecord(f, newSize, "bluray-1080p")
		}
	}
	// Never wait out the production retry backoff. It must not expire during
	// the assertions above, or the test could accidentally cover extra attempts.
	if first.NextRetryAt != nil && !time.Now().Before(*first.NextRetryAt) {
		t.Fatalf("production backoff expired before explicit retry: %+v", first)
	}
	if _, err := db.DB().ExecContext(ctx, `DROP TRIGGER fail_season_episode_registration`); err != nil {
		t.Fatal(err)
	}
	if _, err := wired.importPipeline.RetryJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	final := wait("explicit retry", func(j *imports.ImportJob) bool { return j.Status == imports.JobImported })
	if final.TotalFiles != episodes || len(final.Files) != episodes || final.ProcessedFiles != episodes ||
		final.ImportedFiles != episodes || final.FailedFiles != 0 || final.SkippedFiles != 0 ||
		final.PendingReviewFiles != 0 || final.ReviewID != "" || final.Error != "" {
		t.Fatalf("final outcomes: %+v; files=%+v", final, final.Files)
	}
	finalFiles := make(map[string]imports.ImportJobFile, episodes)
	for _, file := range final.Files {
		before, ok := firstFiles[file.SourcePath]
		if !ok || file.ID != before.ID || file.Status != imports.FileImported || file.Attempts != 1 || file.Error != "" {
			t.Fatalf("retry changed file identity or outcome: before=%+v after=%+v", before, file)
		}
		if _, exists := finalFiles[file.SourcePath]; exists {
			t.Fatalf("duplicate final file outcome: %+v", file)
		}
		finalFiles[file.SourcePath] = file
		if before.Status == imports.FileImported && file != before {
			t.Fatalf("retry reprocessed successful sibling: before=%+v after=%+v", before, file)
		}
	}
	for _, f := range fixtures {
		file, ok := finalFiles[f.source]
		if !ok || file.MediaID != f.id || file.MediaType != string(imports.MediaTypeEpisode) || file.DestPath != f.dest {
			t.Fatalf("%s wrong final linkage: %+v", f.id, file)
		}
		assertSeasonSparseFile(t, f.dest, newSize, f.newMarker, f.newInfo)
		assertRecord(f, newSize, "bluray-1080p")
		if _, err := os.Lstat(f.source); !os.IsNotExist(err) {
			t.Fatalf("%s final move source remains: %v", f.id, err)
		}
	}
	var records, reviews int
	if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_files`).Scan(&records); err != nil || records != episodes {
		t.Fatalf("final episode file rows=%d err=%v", records, err)
	}
	if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM manual_review`).Scan(&reviews); err != nil || reviews != 0 {
		t.Fatalf("clean season produced safety reviews=%d err=%v", reviews, err)
	}
}

// Only write/read small bookend markers; never materialize or hash the sparse
// payload. Retaining FileInfo lets assertions also prove hardlink identity.
func writeSeasonSparseFile(t *testing.T, path string, size int64, marker string) os.FileInfo {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Truncate(path, size); err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int64{0, size - int64(len(marker))} {
		if _, err := f.WriteAt([]byte(marker), offset); err != nil {
			t.Fatal(err)
		}
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func assertSeasonSparseFile(t *testing.T, path string, size int64, marker string, original os.FileInfo) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != size || !os.SameFile(info, original) || !info.ModTime().Equal(original.ModTime()) {
		t.Fatalf("%s size/inode/mtime changed: size=%d want=%d sameFile=%t mtime=%s want=%s",
			path, info.Size(), size, os.SameFile(info, original), info.ModTime(), original.ModTime())
	}
	for _, offset := range []int64{0, size - int64(len(marker))} {
		buf := make([]byte, len(marker))
		if _, err := f.ReadAt(buf, offset); err != nil || string(buf) != marker {
			t.Fatalf("%s marker at %d: got=%q want=%q err=%v", path, offset, buf, marker, err)
		}
	}
}
