package collections

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ebenderooock/loom/internal/metadata/tmdb"
	"github.com/ebenderooock/loom/internal/metadata/tvdb"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

type movieLibrary struct{ items []*movies.Movie }

func (l movieLibrary) ListMovies(context.Context, int, int) ([]*movies.Movie, error) {
	return l.items, nil
}
func (movieLibrary) ListMovieFiles(context.Context, string) ([]*movies.MovieFile, error) {
	return nil, nil
}

type showLibrary struct{ items []*series.Series }

func (l showLibrary) ListSeries(context.Context) ([]*series.Series, error) { return l.items, nil }
func (showLibrary) GetEpisodeStats(context.Context, string) (*series.EpisodeStats, error) {
	return &series.EpisodeStats{}, nil
}

func TestTVDBSourceMixedMembersEnrichExactIDsWithoutCollisions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			_, _ = w.Write([]byte(`{"data":{"token":"test"}}`))
		case "/lists/10/extended":
			_, _ = w.Write([]byte(`{"status":"success","data":{"id":10,"name":"Mixed","entities":[{"movieId":42,"order":1},{"seriesId":42,"order":2}]}}`))
		case "/movies/42/extended":
			_, _ = w.Write([]byte(`{"status":"success","data":{"id":42,"name":"Movie","remoteIds":[{"id":"tt42","sourceName":"IMDB"}]}}`))
		case "/series/42/extended":
			_, _ = w.Write([]byte(`{"status":"success","data":{"id":42,"name":"Show"}}`))
		case "/find/tt42":
			if r.URL.Query().Get("external_source") != "imdb_id" {
				t.Error("wrong IMDb mapping namespace")
			}
			_, _ = w.Write([]byte(`{"movie_results":[{"id":42}],"tv_results":[]}`))
		case "/find/42":
			if r.URL.Query().Get("external_source") != "tvdb_id" {
				t.Error("wrong TVDB mapping namespace")
			}
			_, _ = w.Write([]byte(`{"movie_results":[],"tv_results":[{"id":42}]}`))
		case "/movie/42":
			_, _ = w.Write([]byte(`{"id":42,"title":"Movie","imdb_id":"tt42","release_date":"2020-01-01"}`))
		case "/tv/42":
			_, _ = w.Write([]byte(`{"id":42,"name":"Show","first_air_date":"2021-01-01","external_ids":{"tvdb_id":42,"imdb_id":"tt43"}}`))
		default:
			t.Errorf("unexpected provider path %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s := NewService(testStore(t), tmdb.NewClient(tmdb.Config{APIKey: "test", BaseURL: server.URL}), tvdb.NewClient(tvdb.Config{APIKey: "test", BaseURL: server.URL}), movieLibrary{}, showLibrary{}, nil)
	c, err := s.Import(context.Background(), Source{Provider: "tvdb", Kind: "list", ExternalID: "10"})
	if err != nil || c.Sources[0].Error != "" || len(c.Members) != 2 {
		t.Fatalf("import: %#v %v", c, err)
	}
	for _, m := range c.Members {
		if m.ExternalIDs["tmdb"] != "42" || m.ExternalIDs["tvdb"] != "42" || len(m.Provenance) != 1 {
			t.Fatalf("lost IDs/provenance: %#v", m)
		}
		if m.MediaType == "movie" && m.ExternalIDs["imdb"] != "tt42" {
			t.Fatalf("wrong movie mapping: %#v", m)
		}
		if m.MediaType == "series" && m.ExternalIDs["imdb"] != "tt43" {
			t.Fatalf("wrong show mapping: %#v", m)
		}
	}
}

func TestProviderContradictoryMappingIsNotOverwritten(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "title": "Movie", "imdb_id": "tt99"})
	}))
	defer server.Close()
	s := NewService(nil, tmdb.NewClient(tmdb.Config{APIKey: "test", BaseURL: server.URL}), nil, nil, nil, nil)
	m := Member{MediaType: "movie", Title: "Movie", ExternalIDs: map[string]string{"tmdb": "42", "imdb": "tt42"}}
	if err := s.resolve(context.Background(), &m); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting external IDs accepted: %#v %v", m, err)
	}
}

func TestDiscoveryUsesOnlyDeclaredOfficialCandidatesAndExactSources(t *testing.T) {
	id := "42"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			_, _ = w.Write([]byte(`{"data":{"token":"test"}}`))
		case "/movie/42":
			_, _ = w.Write([]byte(`{"id":42,"title":"Movie","belongs_to_collection":{"id":10,"name":"Same name"}}`))
		case "/series/42/extended":
			_, _ = w.Write([]byte(`{"status":"success","data":{"id":42,"name":"Show","lists":[{"id":10,"name":"Same name","isOfficial":true},{"id":11,"isOfficial":false}]}}`))
		default:
			t.Errorf("unexpected candidate request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store := testStore(t)
	s := NewService(store, tmdb.NewClient(tmdb.Config{APIKey: "test", BaseURL: server.URL}), tvdb.NewClient(tvdb.Config{APIKey: "test", BaseURL: server.URL}), movieLibrary{[]*movies.Movie{{ID: "movie", TMDBID: &id}}}, showLibrary{[]*series.Series{{ID: "show", TVDBID: &id}}}, nil)
	s.fetch = func(_ context.Context, ref Source) (string, []Member, error) {
		return "Same name", []Member{title("movie", "42")}, nil
	}
	items, warnings, err := s.Discover(context.Background())
	if err != nil || len(warnings) != 0 || len(items) != 2 || items[0].ID == items[1].ID {
		t.Fatalf("candidate discovery guessed equivalence: %#v %v %v", items, warnings, err)
	}
	again, _, err := s.Discover(context.Background())
	if err != nil || len(again) != 2 || again[0].ID != items[0].ID {
		t.Fatalf("source duplicate: %#v %v", again, err)
	}
}
