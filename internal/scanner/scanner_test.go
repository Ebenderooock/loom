package scanner

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/series"
)

type importSeriesService struct {
	series.Service
	shows     []*series.Series
	searchErr error
	files     []*series.EpisodeFile
}

func (s *importSeriesService) ListSeries(context.Context) ([]*series.Series, error) {
	return s.shows, nil
}

func (s *importSeriesService) SearchTMDB(context.Context, string) ([]map[string]interface{}, error) {
	return []map[string]interface{}{{"id": float64(1396), "name": "Breaking Bad", "first_air_date": "2008-01-20"}}, s.searchErr
}

func (s *importSeriesService) AddSeries(_ context.Context, req *series.AddSeriesRequest) (*series.Series, error) {
	show := &series.Series{ID: "show", Title: "Breaking Bad", TMDBID: &req.TMDBID}
	s.shows = append(s.shows, show)
	return show, nil
}

func (s *importSeriesService) ListEpisodes(context.Context, string, *int) ([]*series.Episode, error) {
	return []*series.Episode{{ID: "episode", SeriesID: "show", SeasonID: "season", EpisodeNumber: 1}}, nil
}

func (s *importSeriesService) CreateEpisodeFile(_ context.Context, file *series.EpisodeFile) error {
	s.files = append(s.files, file)
	return nil
}

func (s *importSeriesService) UpdateEpisode(context.Context, *series.Episode) error {
	return nil
}

func TestSeriesImportAndMetadataErrors(t *testing.T) {
	for _, failSearch := range []bool{false, true} {
		t.Run(map[bool]string{false: "imports episode", true: "reports metadata failure"}[failSearch], func(t *testing.T) {
			root := t.TempDir()
			folder := filepath.Join(root, "Breaking Bad (2008)", "Season 01")
			if err := os.MkdirAll(folder, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(folder, "Breaking.Bad.S01E01.mkv")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Truncate(60 * 1024 * 1024); err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			svc := &importSeriesService{}
			if failSearch {
				svc.searchErr = errors.New("metadata unavailable")
			}
			scanner := NewSeriesScanner(svc, slog.Default())
			result := &ScanResult{ID: "scan", LibraryID: "library", Status: ScanStatusRunning}
			scanner.scans[result.ID] = result
			scanner.runSeriesScan(context.Background(), result.ID, result.LibraryID, root)
			if failSearch {
				if len(result.Errors) != 1 || result.Errors[0] != `TMDB search for "Breaking Bad": metadata unavailable` {
					t.Fatalf("metadata errors not reported: %+v", result)
				}
				return
			}
			if result.Status != ScanStatusCompleted || result.Imported != 1 || result.TotalFiles != 1 || len(result.Errors) != 0 {
				t.Fatalf("unexpected import result: %+v", result)
			}
			if len(svc.files) != 1 || svc.files[0].FilePath != path {
				t.Fatalf("episode file not persisted: %+v", svc.files)
			}
		})
	}
}
func TestWalkFoldersReportFilesystemErrors(t *testing.T) {
	walkers := map[string]func(string) error{
		"movies": func(root string) error { _, err := walkFolder(root); return err },
		"series": func(root string) error { _, err := walkSeriesFolder(root); return err },
	}
	for name, walk := range walkers {
		t.Run(name, func(t *testing.T) {
			t.Run("missing", func(t *testing.T) {
				err := walk(filepath.Join(t.TempDir(), "missing"))
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("error = %v, want missing directory error", err)
				}
			})
			t.Run("unreadable", func(t *testing.T) {
				if os.Geteuid() == 0 {
					t.Skip("root can read directories without permission")
				}
				root := t.TempDir()
				if err := os.Chmod(root, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := os.Chmod(root, 0700); err != nil {
						t.Error(err)
					}
				})
				if err := walk(root); !errors.Is(err, os.ErrPermission) {
					t.Fatalf("error = %v, want permission error", err)
				}
			})
		})
	}
}
