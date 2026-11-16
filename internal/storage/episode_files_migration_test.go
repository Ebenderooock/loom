package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/ebenderooock/loom/internal/kernel/config"
)

func TestEpisodeFilesMigrationRepairsRepeatedRegistrations(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := openSQLite(ctx, config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "episodes.db")}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	migrations, dialect, err := migrationsFor(EngineSQLite)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.Dialect(dialect), db.DB(), migrations,
		goose.WithLogger(gooseSlogLogger{logger: logger}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 102); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO series (id, title) VALUES ('show', 'Supernatural')`,
		`INSERT INTO seasons (id, series_id, season_number) VALUES ('season', 'show', 10)`,
		`INSERT INTO episodes (id, series_id, season_id, episode_number, has_file) VALUES
		    ('episode', 'show', 'season', 23, 1), ('other', 'show', 'season', 22, 1)`,
	} {
		if _, err := db.DB().ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	dest := filepath.Join(t.TempDir(), "Supernatural - S10E23.mkv")
	if err := os.WriteFile(dest, []byte("one physical episode file"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		if _, err := db.DB().ExecContext(ctx, `INSERT INTO episode_files
		    (id, episode_id, series_id, file_path, file_size, quality, codec, media_info, created_at, updated_at)
		    VALUES (?, 'episode', 'show', ?, ?, 'bluray-1080p', 'x265', '{"duration":2400}', ?, ?)`,
			fmt.Sprintf("file-%02d", i), dest, 100+i,
			"2026-01-01T00:00:00Z", fmt.Sprintf("2026-01-01T00:00:%02dZ", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO episode_files (id, episode_id, series_id, file_path)
	    VALUES ('alternate', 'episode', 'show', '/alternate.mkv'), ('shared', 'other', 'show', ?)`, dest); err != nil {
		t.Fatal(err)
	}
	legacy, err := db.DB().ExecContext(ctx, `UPDATE episode_files SET file_size = file_size WHERE episode_id = 'episode' AND file_path = ?`, dest)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := legacy.RowsAffected(); err != nil || n != 25 {
		t.Fatalf("did not reproduce the reported row count with one physical file: %d, %v", n, err)
	}
	if _, err := provider.UpTo(ctx, 103); err != nil {
		t.Fatal(err)
	}
	var id, quality, codec, mediaInfo string
	var size, count int
	if err := db.DB().QueryRowContext(ctx, `SELECT id, file_size, quality, codec, media_info
	    FROM episode_files WHERE episode_id = 'episode' AND file_path = ?`, dest).
		Scan(&id, &size, &quality, &codec, &mediaInfo); err != nil {
		t.Fatal(err)
	}
	if id != "file-24" || size != 124 || quality != "bluray-1080p" || codec != "x265" || mediaInfo != `{"duration":2400}` {
		t.Fatalf("newest registration was not preserved: %q %d %q %q %s", id, size, quality, codec, mediaInfo)
	}
	if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_files`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("distinct versions or shared episodes changed: count=%d err=%v", count, err)
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO episode_files (id, episode_id, series_id, file_path)
	    VALUES ('duplicate', 'episode', 'show', ?)`, dest); err == nil {
		t.Fatal("duplicate episode/path registration was not prevented")
	}
	result, err := db.DB().ExecContext(ctx, `UPDATE episode_files SET file_size = 999 WHERE episode_id = 'episode' AND file_path = ?`, dest)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("replacement still updates multiple rows: %d, %v", n, err)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "one physical episode file" {
		t.Fatalf("migration touched media content: %q, %v", data, err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migration is not restart-safe: %v", err)
	}
	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 103); err != nil {
		t.Fatalf("migration cannot be reapplied: %v", err)
	}
}
