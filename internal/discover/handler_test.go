package discover

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/metadata/tmdb"
)

func TestVideoDetailsAreReadOnlyAndMediaScoped(t *testing.T) {
	synopsis := strings.Repeat("Full synopsis. ", 150)
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected write: %s", r.Method)
		}
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/movie/42":
			json.NewEncoder(w).Encode(map[string]any{"id": 42, "title": "Movie", "overview": synopsis, "vote_average": 8.5, "runtime": 120})
		case "/tv/42":
			json.NewEncoder(w).Encode(map[string]any{"id": 42, "name": "Series", "overview": synopsis, "number_of_seasons": 3, "vote_average": 9})
		case "/movie/42/credits", "/tv/42/credits":
			json.NewEncoder(w).Encode(map[string]any{"cast": []map[string]any{{"id": 7, "name": "Performer", "character": r.URL.Path}}, "crew": []any{}})
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	router := Router(tmdb.NewClient(tmdb.Config{APIKey: "test", BaseURL: upstream.URL}))
	for _, media := range []string{"movie", "series"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/"+media+"/42", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", media, w.Code, w.Body.String())
		}
		var body struct {
			Title struct {
				Overview string  `json:"overview"`
				TMDBID   string  `json:"tmdb_id"`
				Rating   float64 `json:"rating"`
			} `json:"title"`
			Credits metadata.Credits `json:"credits"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Title.Overview != synopsis || body.Title.TMDBID != "42" || body.Title.Rating == 0 {
			t.Fatalf("incomplete metadata: %+v", body.Title)
		}
		if len(body.Credits.Cast) != 1 || !strings.Contains(body.Credits.Cast[0].Role, "/42/credits") {
			t.Fatalf("missing credits: %+v", body.Credits)
		}
	}
	if strings.Join(paths, ",") != "/movie/42,/movie/42/credits,/tv/42,/tv/42/credits" {
		t.Fatalf("wrong media resolution: %v", paths)
	}
}

func TestExternalIdentityResolutionNeverGuessesByTitle(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/find/tt123":
			if r.URL.Query().Get("external_source") != "imdb_id" {
				t.Error("missing provider scope")
			}
			json.NewEncoder(w).Encode(map[string]any{"movie_results": []map[string]int{{"id": 11}}, "tv_results": []map[string]int{{"id": 22}}})
		case "/tv/22":
			json.NewEncoder(w).Encode(map[string]any{"id": 22, "name": "Show"})
		case "/tv/22/credits":
			json.NewEncoder(w).Encode(map[string]any{"cast": []any{}, "crew": []any{}})
		default:
			t.Errorf("unexpected lookup %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	router := Router(tmdb.NewClient(tmdb.Config{BaseURL: upstream.URL}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/series/tt123?provider=imdb", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"tmdb_id":"22"`) {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	if strings.Join(paths, ",") != "/find/tt123,/tv/22,/tv/22/credits" {
		t.Fatalf("unexpected lookups: %v", paths)
	}
}

func TestInvalidIdentitiesDoNotReachProvider(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid ID reached provider") }))
	defer upstream.Close()
	router := Router(tmdb.NewClient(tmdb.Config{BaseURL: upstream.URL}))
	for _, path := range []string{"/movie/0", "/series/-1", "/movie/nope", "/movie/12?provider=unknown", "/series/tt123?provider=tvdb", "/movie/hello?provider=imdb"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
}

func TestTVDBMovieIdentityFailsExplicitlyWithoutProviderRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unsupported TVDB movie lookup reached TMDB")
	}))
	defer upstream.Close()
	router := Router(tmdb.NewClient(tmdb.Config{BaseURL: upstream.URL}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/movie/42?provider=tvdb", nil))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "IMDb or TMDB mapping") {
		t.Fatalf("unsupported identity: %d %s", w.Code, w.Body)
	}
}

type readOnlyMusic struct{ metadata.MusicMetadataProvider }

func (readOnlyMusic) GetArtist(_ context.Context, id string) (*metadata.ArtistMetadata, error) {
	return &metadata.ArtistMetadata{MBID: id, Name: "Artist", Country: "NZ", Genres: []string{"Jazz"}}, nil
}
func (readOnlyMusic) GetArtistAlbums(context.Context, string) ([]*metadata.AlbumMetadata, error) {
	return []*metadata.AlbumMetadata{{MBID: "b83ae2e8-0c20-48fc-9d8b-2b44d7206e18", Title: "Album", ReleaseDate: "2020-01-01"}}, nil
}
func (readOnlyMusic) GetAlbum(_ context.Context, id string) (*metadata.AlbumMetadata, []*metadata.AlbumReleaseMetadata, error) {
	return &metadata.AlbumMetadata{MBID: id, Title: "Album"}, []*metadata.AlbumReleaseMetadata{{MBID: id, Title: "Edition"}}, nil
}
func (readOnlyMusic) GetAlbumRelease(_ context.Context, id string) (*metadata.AlbumReleaseMetadata, error) {
	return &metadata.AlbumReleaseMetadata{MBID: id, Tracks: []metadata.TrackMetadata{{Title: "Track", TrackNumber: 1}}}, nil
}

func TestMusicDetailsHonorFeatureFlag(t *testing.T) {
	enabled := false
	router := RouterWithMusic(nil, nil, MusicDetails{Provider: readOnlyMusic{}, Enabled: func() bool { return enabled }})
	for _, status := range []int{http.StatusNotFound, http.StatusOK} {
		for _, kind := range []string{"artist", "album", "release"} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/"+kind+"/b83ae2e8-0c20-48fc-9d8b-2b44d7206e18", nil))
			if w.Code != status {
				t.Fatalf("%s enabled=%v: got %d, want %d", kind, enabled, w.Code, status)
			}
		}
		enabled = true
	}
}

func TestMusicMetadataAndTracksWithoutAdding(t *testing.T) {
	router := RouterWithMusic(nil, nil, MusicDetails{Provider: readOnlyMusic{}})
	id := "b83ae2e8-0c20-48fc-9d8b-2b44d7206e18"
	for _, kind := range []string{"artist", "album", "release"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/"+kind+"/"+id, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", kind, w.Code, w.Body.String())
		}

		if !strings.Contains(w.Body.String(), id) {
			t.Errorf("missing identity in %s", kind)
		}
		w = httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/"+kind+"/"+id, nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("write accepted: %d", w.Code)
		}
		w = httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/"+kind+"/invalid", nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("invalid music ID: %d", w.Code)
		}
	}
}

func TestProviderErrorDoesNotExposeCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"status_message":"sensitive provider diagnostics"}`))
	}))
	defer upstream.Close()
	router := Router(tmdb.NewClient(tmdb.Config{BaseURL: upstream.URL}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/movie/42", nil))
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "sensitive") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}

}

func TestConflictingAndAmbiguousVideoIdentitiesAreRejected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/find/tt123":
			json.NewEncoder(w).Encode(map[string]any{"movie_results": []map[string]int{{"id": 11}, {"id": 22}}})
		case "/movie/42":
			json.NewEncoder(w).Encode(map[string]any{"id": 99, "title": "Wrong title"})
		default:
			t.Errorf("conflicting identity triggered further lookup: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()
	router := Router(tmdb.NewClient(tmdb.Config{BaseURL: upstream.URL}))
	for path, status := range map[string]int{"/movie/tt123?provider=imdb": http.StatusNotFound, "/movie/42": http.StatusBadGateway} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestCreditsFailureDoesNotDiscardFullTitleDetails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/movie/42/credits" {
			http.Error(w, "sensitive upstream diagnostics", http.StatusUnauthorized)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{"id": 42, "title": "Movie", "overview": "Full synopsis"})
	}))
	defer upstream.Close()
	router := Router(tmdb.NewClient(tmdb.Config{BaseURL: upstream.URL}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/movie/42", nil))
	var details struct {
		Title        metadata.MovieMetadata `json:"title"`
		CreditsError string                 `json:"credits_error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &details); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || details.Title.Overview != "Full synopsis" || details.CreditsError == "" || strings.Contains(w.Body.String(), "sensitive") {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
}

func TestGrowthRoutesRemainMountedWithReadOnlyDetails(t *testing.T) {
	service, _, _, _ := fixtureService(t)
	router := Router(service.client, service)
	req := httptest.NewRequest(http.MethodGet, "/recommendations", nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), &auth.Identity{
		UserID: 1,
		Roles:  []string{"admin"},
	}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("recommendations route returned %d: %s", w.Code, w.Body.String())
	}
}
