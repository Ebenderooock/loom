package discover

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/metadata/tmdb"
)

func request(router http.Handler, method, path, body string, userID int64) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if userID > 0 {
		req = req.WithContext(auth.WithIdentity(req.Context(), &auth.Identity{UserID: userID, Roles: []string{"admin"}}))
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestGrowthHTTPValidationAndIdentity(t *testing.T) {
	service, _, _, _ := fixtureService(t)
	router := Router(service.client, service)
	for _, path := range []string{
		"/recommendations?media_type=tv", "/recommendations?media_type=",
		"/recommendations?include_upcoming=1", "/recommendations?include_upcoming=",
		"/recommendations?include_upcoming=TRUE", "/collections/0", "/collections/-1", "/collections/nope",
	} {
		w := request(router, "GET", path, "", 1)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s returned %d: %s", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/recommendations", "/collections", "/collections/50"} {
		w := request(router, "GET", path, "", 0)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s returned %d", path, w.Code)
		}
	}
	for _, body := range []string{
		`{}`, `{"tmdb_id":0,"media_type":"movie"}`, `{"tmdb_id":1,"media_type":"tv"}`,
		`{"tmdb_id":-1,"media_type":"series"}`, `{"tmdb_id":2,"media_type":"movie","user_id":2}`,
		`{"tmdb_id":2,"media_type":"movie"} {}`, `invalid`,
	} {
		w := request(router, "POST", "/dismissals", body, 1)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s returned %d", body, w.Code)
		}
	}
	body := `{"tmdb_id":102,"media_type":"movie"}`
	if w := request(router, "POST", "/dismissals", body, 0); w.Code != http.StatusUnauthorized {
		t.Fatal("anonymous dismissal was accepted")
	}
	for range 2 {
		if w := request(router, "POST", "/dismissals", body, 1); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
			t.Fatalf("valid/idempotent dismissal: %d %s", w.Code, w.Body.String())
		}
	}
	for _, user := range []int64{1, 2} {
		w := request(router, "GET", "/recommendations", "", user)
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("recommendations user=%d: %d %s", user, w.Code, w.Body.String())
		}
		var out RecommendationsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Items == nil || out.Warnings == nil || out.GeneratedAt.IsZero() {
			t.Fatalf("invalid envelope: %+v", out)
		}
		found := false
		for _, item := range out.Items {
			found = found || item.TMDBID == 102
			if item.PosterPath != "" && !strings.HasPrefix(item.PosterPath, "https://image.tmdb.org/") {
				t.Fatalf("poster path not a full CDN URL: %q", item.PosterPath)
			}
		}
		if found != (user == 2) {
			t.Fatalf("HTTP dismissal owner isolation broken for user %d", user)
		}
	}
	for _, path := range []string{"/collections", "/collections/50", "/recommendations?media_type=series&include_upcoming=true"} {
		if w := request(router, "GET", path, "", 1); w.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, w.Code, w.Body.String())
		}
	}
	if w := request(router, "GET", "/collections/999", "", 1); w.Code != http.StatusNotFound {
		t.Fatalf("missing collection: %d %s", w.Code, w.Body.String())
	}
}

func TestPersonEndpointPreserved(t *testing.T) {
	service, _, _, _ := fixtureService(t)
	// The original Router(client) signature and person response are preserved.
	router := Router(service.client)
	w := request(router, "GET", "/people/10", "", 1)
	if w.Code != http.StatusOK {
		t.Fatalf("person endpoint returned %d: %s", w.Code, w.Body.String())
	}
	var person PersonFilmography
	if err := json.Unmarshal(w.Body.Bytes(), &person); err != nil || person.Person.ID != 10 || len(person.Credits) == 0 {
		t.Fatalf("person response: %+v, %v", person, err)
	}
	if w := request(router, "GET", "/people/0", "", 1); w.Code != http.StatusBadRequest {
		t.Fatal("person ID validation regressed")
	}
}

func TestHTTPProviderErrorsAreExplicitAndSanitized(t *testing.T) {
	service, fixture, library, _ := fixtureService(t)
	router := Router(service.client, service)
	fixture.fail("/collection/50", http.StatusForbidden)
	w := request(router, "GET", "/collections/50", "", 1)
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "api_key") || strings.Contains(w.Body.String(), "never-echo") {
		t.Fatalf("provider error leaked or misclassified: %d %s", w.Code, w.Body.String())
	}
	fixture.fail("/collection/50", http.StatusOK)
	w = request(router, "GET", "/collections/50", "", 1)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("malformed provider response misclassified: %d %s", w.Code, w.Body.String())
	}
	library.items = library.items[:1] // all seed details fail, not partial success
	fixture.fail("/movie/2", http.StatusUnauthorized)
	w = request(router, "GET", "/recommendations", "", 1)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("total provider failure: %d %s", w.Code, w.Body.String())
	}
	service.client = tmdb.NewClient(tmdb.Config{})
	w = request(router, "GET", "/collections", "", 1)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "not configured") {
		t.Fatalf("missing provider config: %d %s", w.Code, w.Body.String())
	}
}

func TestCacheCoalescesConcurrentReadsAndBoundsQueue(t *testing.T) {
	service, fixture, _, _ := fixtureService(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.Recommendations(context.Background(), 1, "movie", false); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if fixture.count("/movie/1") != 1 || fixture.count("/collection/50") != 1 {
		t.Fatal("concurrent cache misses caused duplicate provider fanout")
	}
	service.gate <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := service.Collection(ctx, 50)
	<-service.gate
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("queued request did not honor cancellation: %v", err)
	}
}

func TestCacheCapacityIsBounded(t *testing.T) {
	service, _, _, _ := fixtureService(t)
	c := service.session()
	for id := range maxCacheItems + 10 {
		_, err := fetch(c, context.Background(), strconv.Itoa(id), func(context.Context) (*int, error) {
			return pointer(id), nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(service.cache) != maxCacheItems {
		t.Fatalf("cache grew beyond capacity: %d", len(service.cache))
	}
}

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
