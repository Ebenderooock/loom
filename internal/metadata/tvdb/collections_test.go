package tvdb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCollectionListsUseSharedAuthAndExtendedTitleEndpoints(t *testing.T) {
	logins := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			logins++
			_, _ = fmt.Fprint(w, `{"data":{"token":"test-token"}}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing shared authentication")
		}
		switch r.URL.Path {
		case "/lists/10/extended":
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"id":10,"name":"Mixed","isOfficial":true,"entities":[{"movieId":42,"order":2},{"seriesId":42,"order":1}]}}`)
		case "/series/42/extended":
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"id":42,"name":"A show","lists":[{"id":10,"isOfficial":true}],"remoteIds":[{"id":"tt42","sourceName":"IMDB"}]}}`)
		case "/movies/42/extended":
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"id":42,"name":"A movie","remoteIds":[{"id":"123","sourceName":"TheMovieDB.com"}]}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
	list, err := client.GetList(context.Background(), 10)
	if err != nil || len(list.Entities) != 2 || list.Entities[0].MovieID != 42 || list.Entities[1].SeriesID != 42 {
		t.Fatalf("mixed list: %#v %v", list, err)
	}
	show, err := client.GetExtendedTitle(context.Background(), "series", 42)
	if err != nil || len(show.Lists) != 1 || !show.Lists[0].IsOfficial || show.RemoteIDs[0].ID != "tt42" {
		t.Fatalf("show candidates: %#v %v", show, err)
	}
	movie, err := client.GetExtendedTitle(context.Background(), "movie", 42)
	if err != nil || movie.RemoteIDs[0].ID != "123" || logins != 1 {
		t.Fatalf("movie mapping/auth: %#v %v %d", movie, err, logins)
	}
}

func TestCollectionListRejectsFailuresAndIncompleteResponses(t *testing.T) {
	for _, body := range []string{`{"status":"success","data":null}`, `{"status":"success","data":{"id":10}}`, `{"status":"success","data":{"id":11,"entities":[]}}`, `{"status":"failure","data":{"id":10,"entities":[]}}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, body) }))
			defer server.Close()
			client := NewClient(Config{APIKey: "test", BaseURL: server.URL})
			client.setToken("test")
			if _, err := client.GetList(context.Background(), 10); err == nil {
				t.Fatal("incomplete source became empty successful list")
			}
		})
	}
}
