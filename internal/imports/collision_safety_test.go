package imports

import (
	"context"
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
	movie *movies.Movie
	files []*movies.MovieFile
	err   error
}

func (s *collisionMoviesService) GetMovie(context.Context, string) (*movies.Movie, error) {
	return s.movie, nil
}
func (s *collisionMoviesService) SearchMovies(context.Context, string) ([]*movies.Movie, error) {
	return []*movies.Movie{s.movie}, nil
}
func (s *collisionMoviesService) ListMovieFiles(context.Context, string) ([]*movies.MovieFile, error) {
	return nil, nil
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
	for _, name := range []string{"part1/video.mkv", "part2/video.mkv", "part2/video.srt"} {
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
	for _, name := range []string{"part1/video.mkv", "part2/video.mkv", "part2/video.srt"} {
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
	if err == nil || !strings.Contains(err.Error(), "file preserved") {
		t.Fatalf("want recoverable library failure, got %v", err)
	}
	got, err := os.ReadFile(filepath.Join(destDir, "video.mkv"))
	if err != nil || string(got) != "only-copy" {
		t.Fatal("library failure deleted the only copy")
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
