package storage

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/ebenderooock/loom/internal/kernel/config"
)

func TestAlbumArtistCreditsMigrationPreservesExistingMusic(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := openSQLite(ctx, config.SQLiteConfig{
		Path: filepath.Join(t.TempDir(), "music.db"),
	}, logger)
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
	if _, err := provider.UpTo(ctx, 99); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO artists (id, name) VALUES ('artist', 'Artist')`,
		`INSERT INTO albums (id, artist_id, title, rating, links)
		 VALUES ('album', 'artist', 'Album', 4.5, '[{"type":"spotify","url":"https://open.spotify.com/album/example"}]')`,
		`INSERT INTO album_artists (album_id, artist_id) VALUES ('album', 'artist')`,
		`INSERT INTO album_releases (id, album_id) VALUES ('release', 'album')`,
		`INSERT INTO tracks (id, album_id, release_id, title, has_file) VALUES ('track', 'album', 'release', 'Song', 1)`,
		`INSERT INTO track_files (id, track_id, album_id, artist_id, file_path)
		 VALUES ('file', 'track', 'album', 'artist', '/music/Artist/Album/Song.flac')`,
	} {
		if _, err := db.DB().ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	assertPreserved := func() {
		t.Helper()
		for _, table := range []string{"albums", "album_artists", "album_releases", "tracks", "track_files"} {
			var count int
			if err := db.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("%s contains %d rows after migration, want 1", table, count)
			}
		}
		var rating float64
		var links string
		if err := db.DB().QueryRowContext(ctx, `SELECT rating, links FROM albums WHERE id='album'`).Scan(&rating, &links); err != nil {
			t.Fatal(err)
		}
		if rating != 4.5 || !strings.Contains(links, "spotify") {
			t.Fatalf("album metadata changed: rating=%v links=%s", rating, links)
		}
		var foreignKeys int
		if err := db.DB().QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
			t.Fatal(err)
		}
		if foreignKeys != 1 {
			t.Fatal("foreign key enforcement was not restored")
		}
		rows, err := db.DB().QueryContext(ctx, `PRAGMA foreign_key_check`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Fatal("migration left invalid foreign keys")
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.UpTo(ctx, 100); err != nil {
		t.Fatal(err)
	}
	assertPreserved()
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO albums (id, title) VALUES ('standalone', 'Standalone')`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Down(ctx); err == nil {
		t.Fatal("downgrade should refuse to discard standalone albums")
	}
	var count int
	if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM albums WHERE id='standalone'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed downgrade changed standalone album: count=%d error=%v", count, err)
	}
	if _, err := db.DB().ExecContext(ctx, `DELETE FROM albums WHERE id='standalone'`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(err)
	}
	assertPreserved()
	if _, err := provider.UpTo(ctx, 100); err != nil {
		t.Fatal(err)
	}
	assertPreserved()
}
