package collections

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

func TestCollectionReadsWorkOfflineAndExposeActualLibraryIdentity(t *testing.T) {
	ctx, store := context.Background(), testStore(t)
	movieID, showID := "42", "42"
	ref := Source{Provider: "imdb", Kind: "list", ExternalID: "ls1", Error: "Access unavailable; retry."}
	movie := title("movie", "42")
	movie.Sources = []string{ref.Key()}
	movie.Provenance = []string{ref.Key()}
	show := title("series", "42")
	show.Sources = []string{ref.Key()}
	show.Provenance = []string{ref.Key()}
	c := saved(t, store, &Collection{Name: "Offline", Sources: []Source{ref}, Members: []Member{movie, show}})
	s := NewService(store, nil, nil, movieLibrary{[]*movies.Movie{{ID: "actual-movie", TMDBID: &movieID, LibraryID: "movie-library", MonitoringStatus: movies.MonitoringStatusUnmonitored}}}, showLibrary{[]*series.Series{{ID: "actual-show", TMDBID: &showID, LibraryID: "show-library", MonitoringStatus: series.MonitoringAll}}}, nil)
	router := Router(s)
	r := httptest.NewRequest(http.MethodGet, "/"+c.ID, nil)
	r = r.WithContext(auth.WithIdentity(r.Context(), &auth.Identity{UserID: 1, Roles: []string{"user"}}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("offline read: %d %s", w.Code, w.Body)
	}
	var result Collection
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.InLibrary != 2 || result.Members[0].MediaID != "actual-movie" || result.Members[1].MediaID != "actual-show" || result.Members[0].Monitored || !result.Members[1].Monitored {
		t.Fatalf("library identity/state: %#v", result)
	}
	if result.Members[0].HasFile == nil || *result.Members[0].HasFile || result.Members[1].HasFile == nil || *result.Members[1].HasFile {
		t.Fatal("library membership implies files")
	}
	stored, err := store.Get(ctx, c.ID)
	if err != nil || stored.Members[0].InLibrary || stored.Members[1].MediaID != "" {
		t.Fatalf("computed state leaked to persistence: %#v %v", stored, err)
	}
}
