package tvmaze

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEpisodesLookupAndBatch(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing user agent")
		}
		switch r.URL.Path {
		case "/lookup/shows":
			http.Redirect(w, r, "/shows/42", http.StatusMovedPermanently)
		case "/shows/42":
			fmt.Fprint(w, `{"id":42}`)
		case "/shows/42/episodes":
			fmt.Fprint(w, `[{"season":1,"number":1,"name":"Pilot","airdate":"2026-10-21","airtime":"20:00","airstamp":"2026-10-22T00:00:00+00:00"}]`)
		default:
			t.Errorf("unexpected path %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := &Client{baseURL: server.URL, http: server.Client()}
	episodes, err := c.Episodes(context.Background(), "123", "tt123")
	if err != nil || len(episodes) != 1 || episodes[0].AirStamp != "2026-10-22T00:00:00+00:00" {
		t.Fatalf("episodes = %+v, err = %v", episodes, err)
	}
	if strings.Join(paths, ",") != "/lookup/shows?thetvdb=123,/shows/42,/shows/42/episodes" {
		t.Fatalf("unexpected requests %v", paths)
	}
}

func TestEpisodesFailuresAndFallback(t *testing.T) {
	for _, status := range []int{404, 429, 500, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Query().Get("imdb") != "tt123" {
					t.Errorf("IMDb lookup not used: %s", r.URL)
				}
				w.WriteHeader(status)
				fmt.Fprint(w, `invalid JSON`)
			}))
			defer server.Close()
			c := &Client{baseURL: server.URL, http: server.Client()}
			eps, err := c.Episodes(context.Background(), "", "tt123")
			if (err == nil) != (status == 404) || len(eps) != 0 || calls != 1 {
				t.Fatalf("eps=%v err=%v calls=%d", eps, err, calls)
			}
		})
	}
}

func TestEpisodesNoIdentityAndCancellation(t *testing.T) {
	c := NewClient()
	if eps, err := c.Episodes(context.Background(), "", ""); err != nil || len(eps) != 0 {
		t.Fatalf("missing IDs: %v, %v", eps, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Episodes(ctx, "123", ""); err == nil {
		t.Fatal("expected cancellation")
	}
}
