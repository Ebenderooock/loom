package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCatalogEndpointsAndQueries(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.URL.Query().Get("api_key") != "fixture-key" {
			t.Error("catalog did not reuse client authentication")
		}
		switch r.URL.Path {
		case "/movie/1":
			if r.URL.Query().Get("append_to_response") != "credits" {
				t.Error("missing appended credits")
			}
			_, _ = w.Write([]byte(`{"id":1,"adult":false,"popularity":42,"genres":[{"id":35,"name":"Comedy"}],"belongs_to_collection":{"id":8,"name":"Set"},"credits":{"cast":[{"id":7,"name":"Actor"}]}}`))
		case "/tv/2":
			if r.URL.Query().Get("append_to_response") != "credits" {
				t.Error("missing TV appended credits")
			}
			_, _ = w.Write([]byte(`{"id":2,"genres":[{"id":35}],"credits":{"crew":[{"id":9,"job":"Director"}]}}`))
		case "/collection/8":
			_, _ = w.Write([]byte(`{"id":8,"name":"Set","overview":"Official set","poster_path":"/set.jpg","parts":[{"id":3,"adult":true,"popularity":8,"release_date":"2030-01-01","genre_ids":[35]}]}`))
		case "/movie/1/recommendations", "/tv/2/recommendations":
			if r.URL.Query().Get("page") != "1" {
				t.Error("recommendations must use one bounded page")
			}
			_, _ = w.Write([]byte(`{"results":[{"id":4,"title":"Related","name":"Related TV","popularity":7,"adult":true}],"page":1}`))
		case "/movie/1/credits", "/tv/2/credits":
			_, _ = w.Write([]byte(`{"id":1,"cast":[{"id":7}],"crew":[{"id":9,"job":"Director"}]}`))
		case "/discover/movie", "/discover/tv":
			q := r.URL.Query()
			if q.Get("include_adult") != "false" || q.Get("page") != "1" || q.Get("with_genres") != "35,10749" || q.Get("sort_by") != "popularity.desc" {
				t.Errorf("incorrect discover query: %v", q)
			}
			key := "primary_release_date.lte"
			if r.URL.Path == "/discover/tv" {
				key = "first_air_date.lte"
			}
			if q.Get(key) != "2026-10-28" {
				t.Errorf("missing date filter %s", key)
			}
			_, _ = w.Write([]byte(`{"results":[{"id":5,"genre_ids":[35,10749],"popularity":99,"adult":false}],"page":1}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(Config{APIKey: "fixture-key", BaseURL: server.URL})
	ctx := context.Background()
	movie, err := client.GetMovieDetails(ctx, 1)
	if err != nil || movie.Collection == nil || movie.Collection.ID != 8 || len(movie.Credits.Cast) != 1 || movie.Popularity != 42 {
		t.Fatalf("movie details = %+v, %v", movie, err)
	}
	tv, err := client.GetTVDetails(ctx, 2)
	if err != nil || len(tv.Credits.Crew) != 1 || len(tv.Genres) != 1 {
		t.Fatalf("TV details = %+v, %v", tv, err)
	}
	set, err := client.GetCollection(ctx, 8)
	if err != nil || set.Overview == "" || !set.Parts[0].Adult || set.Parts[0].GenreIDs[0] != 35 {
		t.Fatalf("collection = %+v, %v", set, err)
	}
	for _, kind := range []string{"movie", "series"} {
		id := 1
		if kind == "series" {
			id = 2
		}
		recs, err := client.GetRecommendations(ctx, kind, id)
		if err != nil || len(recs.Results) != 1 || !recs.Results[0].Adult || recs.Results[0].Popularity != 7 {
			t.Fatalf("recommendations = %+v, %v", recs, err)
		}
		credits, err := client.GetCatalogCredits(ctx, kind, id)
		if err != nil || len(credits.Cast) != 1 || len(credits.Crew) != 1 {
			t.Fatalf("credits = %+v, %v", credits, err)
		}
		discovered, err := client.Discover(ctx, kind, DiscoverOptions{GenreIDs: []int{35, 10749}, ReleasedBy: "2026-10-28"})
		if err != nil || discovered.Results[0].GenreIDs[1] != 10749 {
			t.Fatalf("discover = %+v, %v", discovered, err)
		}
	}
	if len(requests) != 9 {
		t.Fatalf("got %d requests, want 9", len(requests))
	}
	if _, err := client.Discover(ctx, "tv", DiscoverOptions{}); err == nil {
		t.Fatal("invalid public media type accepted")
	}
}

func TestCatalogConfigurationErrorsAndCancellation(t *testing.T) {
	client := NewClient(Config{})
	if client.Configured() {
		t.Fatal("missing key considered configured")
	}
	if _, err := client.GetCollection(context.Background(), 1); err == nil || strings.Contains(err.Error(), "api_key") {
		t.Fatalf("configuration error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/collection/404":
			http.NotFound(w, r)
		case "/collection/401":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	client = NewClient(Config{APIKey: "secret-not-for-errors", BaseURL: server.URL})
	for _, tc := range []struct {
		id   int
		code ErrorCode
	}{{404, ErrCodeNotFound}, {401, ErrCodeUnauthorized}} {
		_, err := client.GetCollection(context.Background(), tc.id)
		var typed *ClientError
		if !errors.As(err, &typed) || typed.Code != tc.code || strings.Contains(err.Error(), "secret-not-for-errors") {
			t.Fatalf("error = %v, want %s", err, tc.code)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := client.GetCollection(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
}

func TestCatalogBearerAuthSurvivesRetry(t *testing.T) {
	token := strings.Repeat("x", 120)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("bearer credential lost on retry")
		}
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"parts":[]}`))
	}))
	defer server.Close()
	client := NewClient(Config{APIKey: token, BaseURL: server.URL})
	if _, err := client.GetCollection(context.Background(), 1); err != nil || calls != 2 {
		t.Fatalf("retry calls=%d, err=%v", calls, err)
	}
}
