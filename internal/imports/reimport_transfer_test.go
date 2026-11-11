package imports

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/series"
)

type transferSeriesService struct {
	series.Service
	files   []*series.EpisodeFile
	profile string
}

func (*transferSeriesService) GetEpisode(context.Context, string) (*series.Episode, error) {
	return &series.Episode{ID: "episode", SeriesID: "show", SeasonID: "season", EpisodeNumber: 3}, nil
}

func (s *transferSeriesService) GetSeries(context.Context, string) (*series.Series, error) {
	return &series.Series{
		ID: "show", Title: "Test Show", LibraryID: "lib", QualityProfileID: s.profile,
		Seasons: []*series.Season{{ID: "season", SeasonNumber: 2}},
	}, nil
}

func (s *transferSeriesService) CreateEpisodeFile(_ context.Context, file *series.EpisodeFile) error {
	s.files = append(s.files, file)
	return nil
}

func TestReimportTransfersToConfiguredMediaLibrary(t *testing.T) {
	for _, mediaType := range []MediaType{MediaTypeMovie, MediaTypeEpisode} {
		for _, mode := range []ImportMode{ImportModeMove, ImportModeCopy, ImportModeHardlink, ImportModeHardlinkOnly} {
			t.Run(string(mediaType)+"/"+string(mode), func(t *testing.T) {
				p, moviesSvc, movieDir := newCollisionPipeline(t, mode)
				seriesSvc := &transferSeriesService{}
				p.matcher.seriesSvc = seriesSvc
				destDir := movieDir
				id := "one"
				if mediaType == MediaTypeEpisode {
					id = "episode"
					destDir = filepath.Join(filepath.Dir(movieDir), "Test Show", "Season 02")
				}
				source := filepath.Join(t.TempDir(), "incoming.mkv")
				if err := os.WriteFile(source, []byte("downloaded media"), 0o600); err != nil {
					t.Fatal(err)
				}
				record, err := p.ReimportFile(context.Background(), mediaType, id, source, ReimportOptions{})
				dest := filepath.Join(destDir, filepath.Base(source))
				if err != nil || record == nil || record.DestPath != dest {
					t.Fatalf("reimport: %+v, %v; expected %s", record, err, dest)
				}
				if got, err := os.ReadFile(dest); err != nil || string(got) != "downloaded media" {
					t.Fatalf("media did not arrive in assigned library: %q, %v", got, err)
				}
				_, err = os.Stat(source)
				if mode == ImportModeMove && !os.IsNotExist(err) {
					t.Fatalf("move left source behind: %v", err)
				}
				if mode != ImportModeMove && err != nil {
					t.Fatalf("%s consumed source: %v", mode, err)
				}
				if mediaType == MediaTypeEpisode {
					if len(seriesSvc.files) != 1 || seriesSvc.files[0].FilePath != dest {
						t.Fatalf("episode not linked to destination: %+v", seriesSvc.files)
					}
				} else if len(moviesSvc.files) != 1 || moviesSvc.files[0].FilePath != dest {
					t.Fatalf("movie not linked to destination: %+v", moviesSvc.files)
				}
			})
		}
	}
}

func TestReimportRegistrationFailurePreservesMoveSource(t *testing.T) {
	p, svc, _ := newCollisionPipeline(t, ImportModeMove)
	svc.err = errors.New("database unavailable")
	source := filepath.Join(t.TempDir(), "incoming.mkv")
	if err := os.WriteFile(source, []byte("only copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ReimportFile(context.Background(), MediaTypeMovie, "one", source, ReimportOptions{}); err == nil {
		t.Fatal("registration failure was hidden")
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "only copy" {
		t.Fatalf("failed registration consumed source: %q, %v", got, err)
	}
}

func TestReimportRefusesItsOwnLibrarySource(t *testing.T) {
	p, _, destDir := newCollisionPipeline(t, ImportModeMove)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(destDir, "existing.mkv")
	if err := os.WriteFile(source, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ReimportFile(context.Background(), MediaTypeMovie, "one", source, ReimportOptions{
		ConflictPolicy: ConflictReplaceAlways,
	}); err == nil {
		t.Fatal("same-path reimport must not remove its source")
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "keep me" {
		t.Fatalf("same-path import changed library file: %q, %v", got, err)
	}
}
