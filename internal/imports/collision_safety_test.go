package imports

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
)

type collisionMoviesService struct {
	movies.Service
	movie    *movies.Movie
	files    []*movies.MovieFile
	existing []*movies.MovieFile
	err      error
}

func (s *collisionMoviesService) GetMovie(context.Context, string) (*movies.Movie, error) {
	return s.movie, nil
}
func (s *collisionMoviesService) SearchMovies(context.Context, string) ([]*movies.Movie, error) {
	return []*movies.Movie{s.movie}, nil
}
func (s *collisionMoviesService) ListMovieFiles(context.Context, string) ([]*movies.MovieFile, error) {
	return s.existing, nil
}
func (s *collisionMoviesService) AddMovieFile(_ context.Context, file *movies.MovieFile) error {
	if s.err != nil {
		return s.err
	}
	s.files = append(s.files, file)
	return nil
}
func (*collisionMoviesService) SetMovieStatus(context.Context, string, movies.MovieStatus) error {
	return nil
}

type uniquePathMoviesService struct {
	*collisionMoviesService
	db *sql.DB
}

func (s *uniquePathMoviesService) AddMovieFile(ctx context.Context, file *movies.MovieFile) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO movie_files(movie_id, file_path, size, quality) VALUES (?, ?, ?, ?)`,
		file.MovieID, file.FilePath, file.Size, file.Quality)
	return err
}

func newCollisionPipeline(t *testing.T, mode ImportMode) (*ImportPipeline, *collisionMoviesService, string) {
	t.Helper()
	opts := basePipelineOptions(t)
	root := t.TempDir()
	_, err := opts.DB.Exec(`CREATE TABLE libraries (
		id TEXT PRIMARY KEY, name TEXT, path TEXT, media_type TEXT,
		monitor_on_add INTEGER, quality_profile_id TEXT,
		unmonitor_on_delete INTEGER, auto_archive_watched INTEGER,
		auto_archive_days_after_watch INTEGER, created_at DATETIME, updated_at DATETIME
	)`)
	if err != nil {
		t.Fatal(err)
	}
	opts.LibStore = libraries.NewStore(opts.DB)
	if err := opts.LibStore.Create(context.Background(), &libraries.Library{
		ID: "lib", Name: "Movies", Path: root, MediaType: "movie",
	}); err != nil {
		t.Fatal(err)
	}
	svc := &collisionMoviesService{movie: &movies.Movie{
		ID: "one", Title: "Safety Film", Year: 2026, LibraryID: "lib",
	}}
	opts.MoviesSvc = svc
	opts.ImportMode = mode
	p, err := NewPipeline(opts)
	if err != nil {
		t.Fatal(err)
	}
	// These tests isolate filesystem collisions, not quality/sample policy.
	p.decisions = NewDecisionMaker()
	return p, svc, filepath.Join(root, "Safety Film (2026)")
}

func TestManualMatchCollisionReturnsExplicitErrorAndPreservesFiles(t *testing.T) {
	for _, mode := range []ImportMode{ImportModeCopy, ImportModeMove, ImportModeHardlink, ImportModeHardlinkOnly} {
		t.Run(string(mode), func(t *testing.T) {
			p, _, destDir := newCollisionPipeline(t, mode)
			source := filepath.Join(t.TempDir(), "video.mkv")
			dest := filepath.Join(destDir, "video.mkv")
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for path, contents := range map[string]string{source: "incoming", dest: "existing"} {
				if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := p.ImportManualMatch(context.Background(), source, MediaTypeMovie, "one")
			if !errors.Is(err, ErrDestinationExists) {
				t.Fatalf("want explicit destination collision, got %v", err)
			}
			for path, want := range map[string]string{source: "incoming", dest: "existing"} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Fatalf("collision changed %s", path)
				}
			}
			var status string
			if err := p.db.QueryRow(`SELECT status FROM import_history LIMIT 1`).Scan(&status); err != nil || status != string(StatusFailed) {
				t.Fatal("collision was not recorded as failed")
			}
		})
	}
}

func TestManualMatchedFolderPreservesNestedLayout(t *testing.T) {
	p, svc, destDir := newCollisionPipeline(t, ImportModeMove)
	src := t.TempDir()
	for _, name := range []string{"part1/video.mkv", "part2/video.mkv", "part2/video.srt", "part2/other.nfo"} {
		path := filepath.Join(src, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.ImportManualMatch(context.Background(), src, MediaTypeMovie, "one"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"part1/video.mkv", "part2/video.mkv", "part2/video.srt", "part2/other.nfo"} {
		got, err := os.ReadFile(filepath.Join(destDir, name))
		if err != nil || string(got) != name {
			t.Fatalf("folder import failed to preserve %s", name)
		}
	}
	if len(svc.files) != 2 || svc.files[0].FilePath == svc.files[1].FilePath {
		t.Fatal("nested media paths were flattened or not registered")
	}
}

func TestManualMatchLibraryFailureKeepsMovedFile(t *testing.T) {
	p, svc, destDir := newCollisionPipeline(t, ImportModeMove)
	svc.err = errors.New("database unavailable")
	src := filepath.Join(t.TempDir(), "video.mkv")
	if err := os.WriteFile(src, []byte("only-copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := p.ImportManualMatch(context.Background(), src, MediaTypeMovie, "one")
	if err == nil || !strings.Contains(err.Error(), "source preserved") {
		t.Fatalf("want recoverable library failure, got %v", err)
	}
	got, err := os.ReadFile(src)
	if err != nil || string(got) != "only-copy" {
		t.Fatal("library failure deleted the only copy")
	}
	if _, err := os.Stat(filepath.Join(destDir, "video.mkv")); !os.IsNotExist(err) {
		t.Fatal("failed registration left destination")
	}
}

func TestImportRecoversMovieFilePathConflict(t *testing.T) {
	for _, tt := range []struct {
		name, owner string
		deleted     bool
		destPresent bool
		wantSuccess bool
	}{
		{name: "deleted matching record", owner: "one", deleted: true, wantSuccess: true},
		{name: "deleted matching record with existing copy", owner: "one", deleted: true, destPresent: true, wantSuccess: true},
		{name: "active matching record", owner: "one", wantSuccess: true},
		{name: "deleted different owner", owner: "other", deleted: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, svc, destDir := newCollisionPipeline(t, ImportModeMove)
			if _, err := p.db.Exec(`CREATE UNIQUE INDEX movie_file_path ON movie_files(file_path)`); err != nil {
				t.Fatal(err)
			}
			dbSvc := &uniquePathMoviesService{collisionMoviesService: svc, db: p.db}
			p.moviesSvc, p.matcher.moviesSvc = dbSvc, dbSvc
			source := filepath.Join(t.TempDir(), "Movie.1080p.WEBRip.mkv")
			dest := filepath.Join(destDir, "Safety Film (2026).mkv")
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, []byte("incoming media"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tt.destPresent {
				if err := os.WriteFile(dest, []byte("incoming media"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var deleted any
			if tt.deleted {
				deleted = "2026-01-01"
			}
			if _, err := p.db.Exec(`INSERT INTO movie_files(movie_id, file_path, size, quality, deleted_at) VALUES (?, ?, 3, 'old', ?)`, tt.owner, dest, deleted); err != nil {
				t.Fatal(err)
			}
			match, err := p.matcher.MatchExact(context.Background(), MediaTypeMovie, "one")
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, source, filepath.Base(dest), true)
			if (err == nil) != tt.wantSuccess {
				t.Fatalf("import error = %v, want success %v", err, tt.wantSuccess)
			}
			var owner, quality string
			var size, count int
			var deletedAt sql.NullString
			if err := p.db.QueryRow(`SELECT movie_id, size, quality, deleted_at FROM movie_files WHERE file_path = ?`, dest).Scan(&owner, &size, &quality, &deletedAt); err != nil {
				t.Fatal(err)
			}
			if err := p.db.QueryRow(`SELECT COUNT(*) FROM movie_files WHERE file_path = ?`, dest).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 || owner != tt.owner {
				t.Fatalf("record ownership or count changed: owner=%q count=%d", owner, count)
			}
			if tt.wantSuccess {
				if deletedAt.Valid || size != len("incoming media") || quality != "webrip-1080p" {
					t.Fatalf("record not restored: deleted=%v size=%d quality=%q", deletedAt, size, quality)
				}
				if data, err := os.ReadFile(dest); err != nil || string(data) != "incoming media" {
					t.Fatalf("destination = %q, %v", data, err)
				}
			} else {
				if !deletedAt.Valid || size != 3 || quality != "old" {
					t.Fatalf("other owner's record changed: deleted=%v size=%d quality=%q", deletedAt, size, quality)
				}
				if data, err := os.ReadFile(source); err != nil || string(data) != "incoming media" {
					t.Fatalf("source not preserved: %q, %v", data, err)
				}
				if _, err := os.Stat(dest); !os.IsNotExist(err) {
					t.Fatalf("failed import left destination: %v", err)
				}
			}
		})
	}
}

func TestPipelineCollisionWithDisabledRecycleBinNeverDeletesExistingFile(t *testing.T) {
	p, _, destDir := newCollisionPipeline(t, ImportModeMove)
	p.recycler = &RecycleBin{Enabled: false}
	src := filepath.Join(t.TempDir(), "Safety.Film.2026.mkv")
	match, err := p.matcher.MatchExact(context.Background(), MediaTypeMovie, "one")
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(destDir, buildDestFilename(match, src))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, contents := range map[string]string{src: "incoming", dest: "existing"} {
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err = p.importSingleFile(context.Background(), &downloads.DownloadCompletedEvent{Title: "Safety Film 2026"}, src)
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("want collision, got %v", err)
	}
	for path, contents := range map[string]string{src: "incoming", dest: "existing"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != contents {
			t.Fatalf("disabled recycle bin caused data loss at %s", path)
		}
	}
}

func (*collisionMoviesService) ListQualityDefinitions(context.Context) ([]*movies.QualityDefinition, error) {
	return nil, nil
}
