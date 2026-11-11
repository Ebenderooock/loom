package series

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestEpisodeFileRegistrationIsIdempotent(t *testing.T) {
	db := monitoringDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	file := &EpisodeFile{
		ID: "original", EpisodeID: "second-ep", SeriesID: "show", FilePath: "/episode.mkv",
		FileSize: 10, Quality: "bluray-1080p", Codec: "x265", MediaInfo: MediaInfoMap{"duration": 2400},
		CreatedAt: created, UpdatedAt: created,
	}
	if err := repo.CreateEpisodeFile(ctx, file); err != nil {
		t.Fatal(err)
	}
	updated := created.Add(time.Hour)
	retry := *file
	retry.ID, retry.FileSize, retry.CreatedAt, retry.UpdatedAt = "retry", 20, updated, updated
	retry.Codec, retry.MediaInfo = "", nil
	if err := repo.CreateEpisodeFile(ctx, &retry); err != nil {
		t.Fatalf("repeat registration failed: %v", err)
	}
	var count, size int
	var id, codec, mediaInfo, createdAt, updatedAt string
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*), id, file_size, codec, media_info, created_at, updated_at
	    FROM episode_files WHERE episode_id = ? AND file_path = ?`, file.EpisodeID, file.FilePath).
		Scan(&count, &id, &size, &codec, &mediaInfo, &createdAt, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if count != 1 || id != "original" || size != 20 || codec != "x265" || mediaInfo != `{"duration":2400}` ||
		createdAt != created.Format(time.RFC3339) || updatedAt != updated.Format(time.RFC3339) {
		t.Fatalf("repeat registration did not update one stable record: %d %q %d %q %s %q %q", count, id, size, codec, mediaInfo, createdAt, updatedAt)
	}
	var hasFile bool
	if err := db.QueryRowContext(ctx, `SELECT has_file FROM episodes WHERE id = ?`, file.EpisodeID).Scan(&hasFile); err != nil || !hasFile {
		t.Fatalf("episode availability was not registered: %v, %v", hasFile, err)
	}
}

func TestConcurrentEpisodeFileRegistrationsDoNotDuplicate(t *testing.T) {
	db := monitoringDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{"first", "retry"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- repo.CreateEpisodeFile(ctx, &EpisodeFile{
				ID: id, EpisodeID: "second-ep", SeriesID: "show", FilePath: "/episode.mkv",
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_files WHERE episode_id = 'second-ep' AND file_path = '/episode.mkv'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("concurrent registrations duplicated a file: count=%d err=%v", count, err)
	}
}

func TestEpisodeFileRegistrationRollsBackWhenAvailabilityUpdateFails(t *testing.T) {
	db := monitoringDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER reject_episode_update BEFORE UPDATE ON episodes
	    BEGIN SELECT RAISE(FAIL, 'availability update failed'); END`); err != nil {
		t.Fatal(err)
	}
	if err := NewRepository(db).CreateEpisodeFile(ctx, &EpisodeFile{
		ID: "file", EpisodeID: "second-ep", SeriesID: "show", FilePath: "/episode.mkv",
	}); err == nil {
		t.Fatal("availability failure was not surfaced")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_files`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed registration left a partial record: count=%d err=%v", count, err)
	}
}
