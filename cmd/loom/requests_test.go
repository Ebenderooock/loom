package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/kernel/scheduler"
	"github.com/ebenderooock/loom/internal/requests"
	"github.com/ebenderooock/loom/internal/storage"
)

func TestRegisterRequestAvailabilityReconcilesAndSchedules(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(ctx, config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "requests.db")},
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store := requests.NewStore(db.DB())
	r, err := store.Create(ctx, requests.Request{UserID: "1", MediaType: requests.MediaMovie, TMDBID: "603", Title: "Movie"})
	if err != nil {
		t.Fatal(err)
	}
	if won, err := store.ClaimForApproval(ctx, r.ID, "admin"); err != nil || !won {
		t.Fatalf("claim: won=%v err=%v", won, err)
	}
	if err := store.MarkApproved(ctx, r.ID, "movie"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO movies(id,title) VALUES ('movie','Movie')`,
		`INSERT INTO movie_files(id,movie_id,file_path) VALUES ('file','movie','/movie.mkv')`,
	} {
		if _, err := db.DB().ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mkv"), []byte("media"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO libraries(id,name,path) VALUES('lib','Movies',?)`, root); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().ExecContext(ctx, `UPDATE movies SET library_id='lib'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().ExecContext(ctx, `UPDATE movie_files SET file_path=?`, filepath.Join(root, "movie.mkv")); err != nil {
		t.Fatal(err)
	}
	sched, err := scheduler.New(scheduler.Config{}, storage.NewSchedulerStore(db), logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := requests.NewService(requests.Options{Store: store, Logger: logger})
	for range 2 {
		if err := registerRequestAvailabilityJob(ctx, sched, svc); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.Get(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != requests.StatusAvailable {
		t.Fatalf("startup did not reconcile approved request: %+v", got)
	}
	if !slices.Contains(sched.JobNames(), "requests-availability") {
		t.Fatalf("missing recurring availability job: %v", sched.JobNames())
	}
}
