package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

type imdbRoundTripper func(*http.Request) (*http.Response, error)

func (f imdbRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func imdbResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestIMDbSource(t *testing.T) {
	for _, tc := range []struct {
		input, root, id string
		wantErr         bool
	}{
		{"ls123", "list", "ls123", false},
		{"https://www.imdb.com/list/ls123/", "list", "ls123", false},
		{"https://imdb.com/list/ls123?sort=date_added&ref_=test", "list", "ls123", false},
		{"https://m.imdb.com/de/list/ls123/#title", "list", "ls123", false},
		{"https://rss.imdb.com/list/ls123", "list", "ls123", false},
		{"https://www.imdb.com/list/ls123/export", "list", "ls123", false},
		{"https://www.imdb.com/user/ur123/watchlist/?ref_=test", "predefinedList", "ur123", false},
		{"https://www.imdb.com/user/p.aBc123/watchlist/", "predefinedList", "p.aBc123", false},
		{"ur123", "predefinedList", "ur123", false},
		{"p.abc123", "predefinedList", "p.abc123", false},
		{"https://feeds.example/list.rss", "", "", false},
		{"https://imdb.com/list/not-a-list/", "", "", true},
		{"https://imdb.com/user/ur123/", "", "", true},
		{"https://imdb.com/title/tt123/", "", "", true},
		{"file:///list/ls123/", "", "", true},
		{"not a URL", "", "", true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			root, id, err := imdbSource(tc.input)
			if (err != nil) != tc.wantErr || root != tc.root || id != tc.id {
				t.Fatalf("imdbSource = (%q, %q, %v), want (%q, %q), error=%v", root, id, err, tc.root, tc.id, tc.wantErr)
			}
		})
	}
}

func TestIMDbFetchPagination(t *testing.T) {
	p := NewIMDbList()
	calls := 0
	p.client.Transport = imdbRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != http.MethodPost || req.URL.String() != imdbGraphQLEndpoint {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		}
		for header, want := range map[string]string{
			"Content-Type": "application/json", "Accept": "application/json", "x-imdb-client-name": "imdb-web-next",
		} {
			if got := req.Header.Get(header); got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		}
		var payload struct {
			Query     string `json:"query"`
			Variables struct {
				ID    string  `json:"id"`
				First int     `json:"first"`
				After *string `json:"after"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Variables.ID != "ls123" || payload.Variables.First != 250 ||
			!strings.Contains(payload.Query, "list(id: $id)") {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		if calls == 1 && payload.Variables.After != nil {
			t.Fatal("first page must not have a cursor")
		}
		if calls == 2 && (payload.Variables.After == nil || *payload.Variables.After != "next") {
			t.Fatal("second page must use the returned cursor")
		}
		if calls > 2 {
			t.Fatal("unexpected extra page")
		}
		// 250 titles on page one, followed by one duplicate and one new series.
		var titles []string
		if calls == 1 {
			for i := 1; i <= 250; i++ {
				titles = append(titles, fmt.Sprintf(`{"title":{"id":"tt%d","titleText":{"text":"Movie %d"},"releaseYear":{"year":2024},"titleType":{"id":"movie"}}}`, i, i))
			}
		} else {
			titles = []string{
				`{"title":{"id":"tt1","titleText":{"text":"Duplicate"}}}`,
				`{"title":{"id":"tt251","titleText":{"text":"Series"},"releaseYear":null,"titleType":{"id":"tvMiniSeries"}}}`,
			}
		}
		return imdbResponse(200, fmt.Sprintf(`{"data":{"list":{"titleListItemSearch":{"edges":[%s],"pageInfo":{"hasNextPage":%t,"endCursor":"next"}}}}}`, strings.Join(titles, ","), calls == 1)), nil
	})
	items, err := p.Fetch(context.Background(), ProviderConfig{URL: "  https://www.imdb.com/list/ls123/?ref_=test  "})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(items) != 251 {
		t.Fatalf("got %d requests and %d items, want 2 and 251", calls, len(items))
	}
	if want := (Item{ExternalID: "tt1", IMDbID: "tt1", Title: "Movie 1", Year: 2024, MediaType: "movie"}); items[0] != want {
		t.Errorf("first item = %+v, want %+v", items[0], want)
	}
	if want := (Item{ExternalID: "tt251", IMDbID: "tt251", Title: "Series", MediaType: "series"}); items[250] != want {
		t.Errorf("last item = %+v, want %+v", items[250], want)
	}
}

func TestIMDbWatchlist(t *testing.T) {
	for _, id := range []string{"ur123", "p.abc123"} {
		t.Run(id, func(t *testing.T) {
			p := NewIMDbWatchlist()
			calls := 0
			p.client.Transport = imdbRoundTripper(func(req *http.Request) (*http.Response, error) {
				calls++
				var payload struct {
					Query     string `json:"query"`
					Variables struct {
						ID string `json:"id"`
					} `json:"variables"`
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				if id == "p.abc123" && calls == 1 {
					if payload.Variables.ID != id || !strings.Contains(payload.Query, "userProfile") {
						t.Fatalf("unexpected profile query: %s", body)
					}
					return imdbResponse(200, `{"data":{"userProfile":{"userId":"ur123"}}}`), nil
				}
				if payload.Variables.ID != "ur123" || !strings.Contains(payload.Query, "predefinedList(classType: WATCH_LIST") {
					t.Fatalf("unexpected watchlist query: %s", body)
				}
				return imdbResponse(200, `{"data":{"predefinedList":{"titleListItemSearch":{"edges":[],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}`), nil
			})
			items, err := p.Fetch(context.Background(), ProviderConfig{URL: "https://www.imdb.com/user/" + id + "/watchlist/"})
			if err != nil || items == nil || len(items) != 0 {
				t.Fatalf("empty public watchlist = (%v, %v)", items, err)
			}
			wantCalls := 1
			if id == "p.abc123" {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatalf("got %d requests, want %d", calls, wantCalls)
			}
		})
	}
}

func TestIMDbFetchErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"private", `{"errors":[{"message":"FORBIDDEN"}],"data":{"list":null}}`, "public", 200},
		{"missing", `{"data":{"list":null}}`, "not found", 200},
		{"no data", `{}`, "missing response data", 200},
		{"incomplete", `{"data":{"list":{}}}`, "incomplete", 200},
		{"missing edges", `{"data":{"list":{"titleListItemSearch":{"pageInfo":{"hasNextPage":false}}}}}`, "incomplete", 200},
		{"invalid title", `{"data":{"list":{"titleListItemSearch":{"edges":[{"title":null}],"pageInfo":{"hasNextPage":false}}}}}`, "invalid title", 200},
		{"bad json", `<html>challenge</html>`, "parse response", 200},
		{"forbidden", "", "403", 403},
		{"rate limit", "", "429", 429},
		{"no cursor", `{"data":{"list":{"titleListItemSearch":{"edges":[],"pageInfo":{"hasNextPage":true}}}}}`, "pagination", 200},
		{"repeated cursor", `{"data":{"list":{"titleListItemSearch":{"edges":[{"title":{"id":"tt1"}}],"pageInfo":{"hasNextPage":true,"endCursor":"same"}}}}}`, "pagination", 200},
		{"oversized", strings.Repeat(" ", (10<<20)+1), "exceeds", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := NewIMDbList()
			calls := 0
			p.client.Transport = imdbRoundTripper(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls > 2 {
					t.Fatal("pagination did not terminate")
				}
				return imdbResponse(tc.status, tc.body), nil
			})
			items, err := p.Fetch(context.Background(), ProviderConfig{URL: "ls123"})
			if err == nil || !strings.Contains(err.Error(), tc.want) || items != nil {
				t.Fatalf("Fetch = (%v, %v), want nil items and error containing %q", items, err, tc.want)
			}
		})
	}
}

func TestIMDbFetchRSS(t *testing.T) {
	p := NewIMDbList()
	p.client.Transport = imdbRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.String() != "https://feeds.example/list.rss" {
			t.Fatalf("unexpected RSS request: %s %s", req.Method, req.URL)
		}
		return imdbResponse(200, `<rss><channel><item><title>Example (2020)</title><link>https://imdb.com/title/tt123/</link></item></channel></rss>`), nil
	})
	items, err := p.Fetch(context.Background(), ProviderConfig{URL: "https://feeds.example/list.rss"})
	want := []Item{{ExternalID: "tt123", IMDbID: "tt123", Title: "Example", Year: 2020}}
	if err != nil || !reflect.DeepEqual(items, want) {
		t.Fatalf("Fetch = (%v, %v), want %v", items, err, want)
	}
}

func TestIMDbFetchCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewIMDbList().Fetch(ctx, ProviderConfig{URL: "ls123"}); err == nil {
		t.Fatal("expected cancelled request to fail")
	}
}

func TestIMDbLive(t *testing.T) {
	if os.Getenv("LOOM_TEST_IMDB_LIVE") != "1" {
		t.Skip("set LOOM_TEST_IMDB_LIVE=1 to check the public IMDb API")
	}
	items, err := NewIMDbList().Fetch(context.Background(), ProviderConfig{URL: "https://www.imdb.com/list/ls004285275/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("expected titles from the public list")
	}
	for _, item := range items {
		if !imdbTitleID.MatchString(item.IMDbID) || item.ExternalID != item.IMDbID || item.Title == "" {
			t.Fatalf("invalid item: %+v", item)
		}
	}
	t.Logf("fetched %d titles", len(items))
}
