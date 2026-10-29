package tmdb

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFindMovieByIMDbID(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
		wantErr          bool
	}{
		{"mapped", `{"movie_results":[{"id":550}]}`, "550", 200, false},
		{"unmapped", `{"movie_results":[],"tv_results":[{"id":550}]}`, "", 200, false},
		{"ambiguous", `{"movie_results":[{"id":550},{"id":551}]}`, "", 200, true},
		{"invalid id", `{"movie_results":[{"id":0}]}`, "", 200, true},
		{"invalid response", `{`, "", 200, true},
		{"provider failure", `{}`, "", 403, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/find/tt0137523" || r.URL.Query().Get("external_source") != "imdb_id" {
					t.Errorf("unexpected request %s", r.URL)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client := NewClient(Config{APIKey: "test", BaseURL: server.URL})
			id, err := client.FindMovieByIMDbID(context.Background(), "tt0137523")
			if id != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("FindMovieByIMDbID = (%q, %v), want (%q, error=%t)", id, err, tc.want, tc.wantErr)
			}
		})
	}
}
