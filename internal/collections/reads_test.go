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

type statusShows struct {
	showLibrary
	stats *series.EpisodeStats
}

func (s statusShows) GetEpisodeStats(context.Context, string) (*series.EpisodeStats, error) {
	return s.stats, nil
}

func TestCollectionReadsUseLiveLibraryStatusNotFilePresence(t *testing.T) {
	ctx, store := context.Background(), testStore(t)
	id := "42"
	movie := &movies.Movie{ID: "local-movie", TMDBID: &id, MonitoringStatus: movies.MonitoringStatusUnmonitored}
	stats := &series.EpisodeStats{AiredEpisodes: 8, DownloadedEpisodes: 3, MissingEpisodes: 5}
	shows := statusShows{showLibrary{[]*series.Series{{ID: "local-show", TMDBID: &id, Status: series.SeriesStatus("continuing"), MonitoringStatus: series.MonitoringAll}}}, stats}
	c := saved(t, store, &Collection{Name: "Live status", Members: []Member{title("movie", id), title("series", id), title("movie", "99")}})
	s := NewService(store, nil, nil, movieLibrary{[]*movies.Movie{movie}}, shows, nil)
	router := Router(s)
	read := func(path string) Collection {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r = r.WithContext(auth.WithIdentity(r.Context(), &auth.Identity{UserID: 1, Roles: []string{"user"}}))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("read: %d %s", w.Code, w.Body)
		}
		var got Collection
		if path == "/" {
			var list struct {
				Items []Collection `json:"items"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Items) != 1 {
				t.Fatalf("list: %s %v", w.Body, err)
			}
			got = list.Items[0]
		} else if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	for _, status := range []movies.MovieStatus{
		movies.MovieStatusMissing, movies.MovieStatusDownloading, movies.MovieStatusStoring,
		movies.MovieStatusAvailableRightQuality, movies.MovieStatusAvailableWrongQuality,
		movies.MovieStatusAvailableHigherQuality, movies.MovieStatusUnreleased,
	} {
		movie.Status = status
		for _, path := range []string{"/", "/" + c.ID} {
			got := read(path)
			m, show, external := got.Members[0], got.Members[1], got.Members[2]
			if m.Status != string(status) || m.MonitoringStatus != string(movie.MonitoringStatus) || m.Monitored {
				t.Fatalf("movie status lost/overridden: %+v", m)
			}
			if show.Status != "continuing" || show.EpisodeStats == nil || *show.EpisodeStats != *stats {
				t.Fatalf("series episode aggregation differs: %+v", show)
			}
			if external.Status != "" || external.EpisodeStats != nil || external.InLibrary {
				t.Fatalf("external member claims library state: %+v", external)
			}
		}
	}
	stats.DownloadedEpisodes, stats.MissingEpisodes = 8, 0
	got := read("/" + c.ID)
	if got.Members[1].EpisodeStats.DownloadedEpisodes != 8 {
		t.Fatal("episode state cached instead of refreshed")
	}
	if err := store.Save(ctx, &got, false); err != nil {
		t.Fatal(err)
	}
	stored, err := store.Get(ctx, c.ID)
	if err != nil || stored.Members[0].Status != "" || stored.Members[1].EpisodeStats != nil || stored.Members[0].MonitoringStatus != "" {
		t.Fatalf("computed status persisted: %+v %v", stored, err)
	}
}
