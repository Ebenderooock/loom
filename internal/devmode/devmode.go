package devmode

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
)

//go:embed testdata
var fixtures embed.FS

// Stubs holds the base URLs of all running stub servers. Pass the fields
// into the corresponding client Config structs to redirect outbound calls.
type Stubs struct {
	TMDbURL        string
	TVDbURL        string
	MusicBrainzURL string
}

// Start starts all stub servers and returns their base URLs plus a stop
// function. Callers must call stop() (typically via defer) when done.
func Start() (*Stubs, func()) {
	tmdb := newTMDbServer()
	tvdb := newTVDbServer()
	mb := newMusicBrainzServer()

	stop := func() {
		tmdb.Close()
		tvdb.Close()
		mb.Close()
	}

	return &Stubs{
		TMDbURL:        tmdb.URL,
		TVDbURL:        tvdb.URL,
		MusicBrainzURL: mb.URL,
	}, stop
}

// fixture reads a testdata file and returns its bytes. Panics on missing
// files so misconfigured stubs fail loudly at startup.
func fixture(name string) []byte {
	b, err := fs.ReadFile(fixtures, "testdata/"+name)
	if err != nil {
		panic("devmode: missing fixture " + name + ": " + err.Error())
	}
	return b
}

// writeJSON writes a JSON body with status 200.
func writeJSON(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// newTMDbServer returns a stub that handles the TMDb v3 endpoints used by
// internal/metadata/tmdb.
func newTMDbServer() *httptest.Server {
	mux := http.NewServeMux()

	// /movie/{id} and /movie/{id}/credits and /movie/{id}/release_dates
	mux.HandleFunc("/movie/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/credits"):
			writeJSON(w, fixture("tmdb_credits.json"))
		case strings.HasSuffix(path, "/release_dates"):
			writeJSON(w, fixture("tmdb_release_dates.json"))
		default:
			writeJSON(w, fixture("tmdb_movie.json"))
		}
	})

	// /tv/{id} and /tv/{id}/season/{s}/episode/{e}
	mux.HandleFunc("/tv/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, fixture("tmdb_tv.json"))
	})

	// /search/movie
	mux.HandleFunc("/search/movie", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, fixture("tmdb_search_movie.json"))
	})

	// /search/tv
	mux.HandleFunc("/search/tv", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, fixture("tmdb_search_tv.json"))
	})

	// /person/{id}
	mux.HandleFunc("/person/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":                    1,
			"name":                  "Dev Person",
			"biography":             "",
			"profile_path":          "",
			"known_for_department":  "Acting",
		})
	})

	// /configuration — some clients call this for image base URLs
	mux.HandleFunc("/configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"images": map[string]interface{}{
				"base_url":        "http://image.tmdb.org/t/p/",
				"secure_base_url": "https://image.tmdb.org/t/p/",
				"poster_sizes":    []string{"w92", "w154", "w185", "w342", "w500", "w780", "original"},
			},
		})
	})

	// Catch-all: return empty 200 so unknown endpoints don't crash startup.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	return httptest.NewServer(mux)
}

// newTVDbServer returns a stub that handles the TVDb v4 endpoints used by
// internal/metadata/tvdb.
func newTVDbServer() *httptest.Server {
	mux := http.NewServeMux()

	// POST /login
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, fixture("tvdb_login.json"))
	})

	// /series/{id}/episodes/{season-type}
	mux.HandleFunc("/series/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/episodes/") {
			writeJSON(w, fixture("tvdb_episodes.json"))
			return
		}
		writeJSON(w, fixture("tvdb_series.json"))
	})

	// /search
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, fixture("tvdb_search.json"))
	})

	// Catch-all
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	return httptest.NewServer(mux)
}

// newMusicBrainzServer returns a stub that handles the MusicBrainz WS2
// endpoints used by internal/metadata/musicbrainz.
func newMusicBrainzServer() *httptest.Server {
	mux := http.NewServeMux()

	// /artist/{mbid} or /artist?query=...
	mux.HandleFunc("/artist", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "" {
			writeJSON(w, fixture("musicbrainz_search_artist.json"))
			return
		}
		writeJSON(w, fixture("musicbrainz_artist.json"))
	})
	mux.HandleFunc("/artist/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, fixture("musicbrainz_artist.json"))
	})

	// /release/{mbid} or /release?query=...
	mux.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "" {
			// Return search response shape
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count":    1,
				"offset":   0,
				"releases": []interface{}{json.RawMessage(fixture("musicbrainz_release.json"))},
			})
			return
		}
		writeJSON(w, fixture("musicbrainz_release.json"))
	})
	mux.HandleFunc("/release/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, fixture("musicbrainz_release.json"))
	})

	// /recording/{mbid}
	mux.HandleFunc("/recording/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    "rec-001",
			"title": "Dev Recording",
		})
	})

	// Catch-all
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	return httptest.NewServer(mux)
}
