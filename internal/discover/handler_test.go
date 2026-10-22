package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/metadata/tmdb"
)

func TestMediaDetails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/42":
			_, _ = w.Write([]byte(`{"id":42,"title":"Movie","overview":"Full movie overview","runtime":123,"release_date":"2026-01-01","genres":[{"name":"Drama"}],"vote_average":8}`))
		case "/tv/43":
			_, _ = w.Write([]byte(`{"id":43,"name":"Show","overview":"Full show overview","first_air_date":"2026-01-01","number_of_seasons":3}`))
		default:
			http.Error(w, "upstream unavailable", http.StatusUnauthorized)
		}
	}))
	defer upstream.Close()
	router := Router(tmdb.NewClient(tmdb.Config{APIKey: "test", BaseURL: upstream.URL, Timeout: time.Second}))
	for _, tc := range []struct {
		path  string
		code  int
		title string
	}{
		{"/movie/42", 200, "Movie"},
		{"/series/43", 200, "Show"},
		{"/movie/nope", 400, ""},
		{"/series/0", 400, ""},
		{"/movie/99", 502, ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != tc.code {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if tc.code == 200 {
				var data map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
					t.Fatal(err)
				}
				if data["title"] != tc.title || data["overview"] == "" {
					t.Fatalf("unexpected details: %v", data)
				}
			}
		})
	}
}
