package organizer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/movies"
)

type organizerMoviesStub struct {
	movie       *movies.Movie
	files       []*movies.MovieFile
	libraryPath string
}

func (s *organizerMoviesStub) GetMovie(_ context.Context, id string) (*movies.Movie, error) {
	if s.movie == nil || s.movie.ID != id {
		return nil, errors.New("not found")
	}
	return s.movie, nil
}
func (s *organizerMoviesStub) ListMovies(_ context.Context, _, _ int) ([]*movies.Movie, error) {
	if s.movie == nil {
		return nil, nil
	}
	return []*movies.Movie{s.movie}, nil
}
func (s *organizerMoviesStub) ListMovieFiles(_ context.Context, movieID string) ([]*movies.MovieFile, error) {
	if s.movie == nil || s.movie.ID != movieID {
		return nil, errors.New("not found")
	}
	return s.files, nil
}
func (s *organizerMoviesStub) GetLibraryPath(_ context.Context, libraryID string) (string, error) {
	if s.movie == nil || s.movie.LibraryID != libraryID {
		return "", errors.New("not found")
	}
	return s.libraryPath, nil
}

type organizerUpdaterStub struct {
	updated []*movies.MovieFile
}

func (s *organizerUpdaterStub) UpdateMovieFile(_ context.Context, mf *movies.MovieFile) error {
	s.updated = append(s.updated, mf)
	return nil
}

type organizerConfigStub struct {
	cfg *NamingConfig
}

func (s *organizerConfigStub) GetNamingConfig(context.Context) (*NamingConfig, error) {
	return s.cfg, nil
}
func (s *organizerConfigStub) SaveNamingConfig(context.Context, *NamingConfig) error { return nil }

func testOrganizerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNamingFormatAndTargetPath(t *testing.T) {
	t.Parallel()
	m := &movies.Movie{
		Title: "The Matrix: Reloaded",
		Year:  2003,
	}
	f := &movies.MovieFile{
		FilePath: "/tmp/The.Matrix.Reloaded.2003.1080p.BluRay.x264-GROUP.mkv",
		Quality:  "Bluray-1080p",
	}

	tests := []struct {
		name       string
		cfg        *NamingConfig
		wantFolder string
		wantFile   string
	}{
		{
			name:       "default format",
			cfg:        DefaultNamingConfig(),
			wantFolder: "The Matrix - Reloaded (2003)",
			wantFile:   "The Matrix - Reloaded (2003) [Bluray-1080p]",
		},
		{
			name: "clean title and source",
			cfg: &NamingConfig{
				MovieFolderFormat: "{Movie CleanTitle}",
				MovieFileFormat:   "{Movie TitleThe} {Quality Source} {Quality Resolution}",
				ColonReplacement:  "-",
			},
			wantFolder: "matrix reloaded",
			wantFile:   "Matrix- Reloaded, The BluRay 1080p",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folder := FormatFolderName(m, tt.cfg)
			if folder != tt.wantFolder {
				t.Fatalf("folder = %q, want %q", folder, tt.wantFolder)
			}

			name := FormatFileName(m, f, tt.cfg)
			if name != tt.wantFile {
				t.Fatalf("file = %q, want %q", name, tt.wantFile)
			}

			target := BuildTargetPath("/library", m, f, tt.cfg)
			if !strings.HasSuffix(target, filepath.Join(tt.wantFolder, tt.wantFile+".mkv")) {
				t.Fatalf("unexpected target path %q", target)
			}
		})
	}
}

func TestOrganizeMovie_MoveMode(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "incoming")
	library := filepath.Join(tmp, "library")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.MkdirAll(library, 0o755); err != nil {
		t.Fatalf("mkdir library: %v", err)
	}

	src := filepath.Join(srcDir, "The.Matrix.1999.1080p.BluRay.mkv")
	if err := os.WriteFile(src, []byte("video"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	mv := &movies.Movie{ID: "m1", Title: "The Matrix", Year: 1999, LibraryID: "lib1"}
	mf := &movies.MovieFile{ID: "f1", MovieID: "m1", FilePath: src, Quality: "Bluray-1080p"}

	mp := &organizerMoviesStub{movie: mv, files: []*movies.MovieFile{mf}, libraryPath: library}
	up := &organizerUpdaterStub{}
	cs := &organizerConfigStub{cfg: DefaultNamingConfig()}
	org := New(mp, up, cs, testOrganizerLogger())

	results, err := org.OrganizeMovie(context.Background(), "m1")
	if err != nil {
		t.Fatalf("OrganizeMovie: %v", err)
	}
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("unexpected organize result: %+v", results)
	}

	newPath := results[0].NewPath
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected destination file: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be moved, stat err=%v", err)
	}
	if len(up.updated) != 1 || up.updated[0].FilePath != newPath {
		t.Fatalf("movie file record not updated: %+v", up.updated)
	}
}

func TestOrganizeMovie_HardlinkMode(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "incoming")
	library := filepath.Join(tmp, "library")
	_ = os.MkdirAll(srcDir, 0o755)
	_ = os.MkdirAll(library, 0o755)

	src := filepath.Join(srcDir, "The.Matrix.1999.1080p.BluRay.mkv")
	if err := os.WriteFile(src, []byte("video"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	mv := &movies.Movie{ID: "m1", Title: "The Matrix", Year: 1999, LibraryID: "lib1"}
	mf := &movies.MovieFile{ID: "f1", MovieID: "m1", FilePath: src, Quality: "Bluray-1080p"}

	mp := &organizerMoviesStub{movie: mv, files: []*movies.MovieFile{mf}, libraryPath: library}
	up := &organizerUpdaterStub{}
	cs := &organizerConfigStub{cfg: DefaultNamingConfig()}
	org := New(mp, up, cs, testOrganizerLogger())
	org.SetImportMode("hardlink")

	results, err := org.OrganizeMovie(context.Background(), "m1")
	if err != nil {
		t.Fatalf("OrganizeMovie: %v", err)
	}
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("unexpected organize result: %+v", results)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should remain in hardlink mode: %v", err)
	}
	if _, err := os.Stat(results[0].NewPath); err != nil {
		t.Fatalf("expected destination file: %v", err)
	}
}

func TestOrganizeMovie_RenameDisabled(t *testing.T) {
	t.Parallel()
	mv := &movies.Movie{ID: "m1", Title: "Movie", Year: 2000, LibraryID: "lib1"}
	mp := &organizerMoviesStub{movie: mv, libraryPath: t.TempDir()}
	up := &organizerUpdaterStub{}
	cs := &organizerConfigStub{cfg: &NamingConfig{
		ID:                "default",
		MovieFolderFormat: "{Movie Title}",
		MovieFileFormat:   "{Movie Title}",
		ColonReplacement:  " -",
		RenameMovies:      false,
	}}
	org := New(mp, up, cs, testOrganizerLogger())

	if _, err := org.OrganizeMovie(context.Background(), "m1"); err == nil {
		t.Fatal("expected error when rename_movies is disabled")
	}
}
