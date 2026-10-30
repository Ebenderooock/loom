package providers

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestMixedTMDBListPaginationAndMediaIdentity(t *testing.T) {
	p := NewTMDbMixedList()
	requests := 0
	p.client.Transport = imdbRoundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host != "api.themoviedb.org" || r.URL.Path != "/4/list/123" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("wrong source request: %s", r.URL)
		}
		switch r.URL.Query().Get("page") {
		case "1":
			return imdbResponse(200, `{"total_pages":2,"results":[{"id":42,"media_type":"movie","title":"A movie","release_date":"2020-01-01","poster_path":"/movie.jpg"}]}`), nil
		case "2":
			return imdbResponse(200, `{"total_pages":2,"results":[{"id":42,"media_type":"tv","name":"A show","first_air_date":"2021-01-01","poster_path":"/show.jpg"}]}`), nil
		default:
			return nil, fmt.Errorf("unexpected page")
		}
	})
	items, err := p.Fetch(context.Background(), ProviderConfig{URL: "123", APIKey: "test-token"})
	if err != nil || len(items) != 2 || requests != 2 {
		t.Fatalf("list: %#v %v", items, err)
	}
	if items[0].MediaType != "movie" || items[1].MediaType != "series" || items[0].TMDbID != items[1].TMDbID || items[1].Year != 2021 || items[1].PosterPath != "/show.jpg" {
		t.Fatalf("namespace lost: %#v", items)
	}
}

func TestMixedTMDBListDoesNotSwallowLaterPageAccessFailure(t *testing.T) {
	p := NewTMDbMixedList()
	p.client.Transport = imdbRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("page") == "1" {
			return imdbResponse(200, `{"total_pages":2,"results":[{"id":1,"media_type":"movie","title":"Movie"}]}`), nil
		}
		return imdbResponse(403, `private source data`), nil
	})
	items, err := p.Fetch(context.Background(), ProviderConfig{URL: "123", APIKey: "test"})
	if err == nil || items != nil {
		t.Fatalf("partial success: %#v %v", items, err)
	}
}

func TestMixedTMDBListRejectsArbitraryURLs(t *testing.T) {
	p := NewTMDbMixedList()
	p.client.Transport = imdbRoundTripper(func(*http.Request) (*http.Response, error) { t.Fatal("invalid source reached HTTP"); return nil, nil })
	if _, err := p.Fetch(context.Background(), ProviderConfig{URL: "https://localhost/", APIKey: "test"}); err == nil {
		t.Fatal("accepted arbitrary URL")
	}
}
