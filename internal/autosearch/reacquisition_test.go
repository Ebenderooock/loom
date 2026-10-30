package autosearch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ebenderooock/loom/internal/movies"
)

type recordedMovieFileService struct {
	movies.Service
}

func (*recordedMovieFileService) ListMovieFiles(context.Context, string) ([]*movies.MovieFile, error) {
	return []*movies.MovieFile{{MovieID: "movie", FilePath: "/stale.mkv", Quality: "Bluray-1080p"}}, nil
}

func TestConfirmedMissingAcquisitionDoesNotTreatStaleFilesAsUpgrades(t *testing.T) {
	e := &Engine{movieSvc: &recordedMovieFileService{}}
	req := SearchRequest{MediaType: "movie", MediaID: "movie"}
	if !e.getExistingQuality(context.Background(), req, nil, nil).HasFile {
		t.Fatal("normal searches must retain their existing-file guard")
	}
	req.ConfirmedMissing = true
	if e.getExistingQuality(context.Background(), req, nil, nil).HasFile {
		t.Fatal("confirmed missing files must not be treated as upgrades")
	}
	var clientInput SearchRequest
	if err := json.Unmarshal([]byte(`{"media_type":"movie","ConfirmedMissing":true,"confirmed_missing":true}`), &clientInput); err != nil {
		t.Fatal(err)
	}
	if clientInput.ConfirmedMissing {
		t.Fatal("clients must not bypass file guards")
	}
}
