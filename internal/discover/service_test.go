package discover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/metadata/tmdb"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/storage"
)

type movieLibrary struct {
	items    []*movies.Movie
	files    map[string][]*movies.MovieFile
	fileErr  error
	listErr  error
	fileGets int
}

func (m *movieLibrary) ListMovies(ctx context.Context, limit, offset int) ([]*movies.Movie, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.listErr != nil {
		return nil, m.listErr
	}
	if offset >= len(m.items) {
		return nil, nil
	}
	end := offset + limit
	if end > len(m.items) {
		end = len(m.items)
	}
	return m.items[offset:end], nil
}

func (m *movieLibrary) ListMovieFiles(_ context.Context, id string) ([]*movies.MovieFile, error) {
	m.fileGets++
	return m.files[id], m.fileErr
}

type seriesLibrary struct{ items []*series.Series }

func (s *seriesLibrary) ListSeries(context.Context) ([]*series.Series, error) { return s.items, nil }

func pointer[T any](value T) *T { return &value }

func libraryMovie(id int) *movies.Movie {
	return &movies.Movie{ID: fmt.Sprintf("movie-%d", id), TMDBID: pointer(strconv.Itoa(id)),
		Title: fmt.Sprintf("Library %d", id), MonitoringStatus: movies.MonitoringStatusMonitored}
}

func openStore(t *testing.T, path string) (storage.DB, *Store) {
	t.Helper()
	db, err := storage.Open(context.Background(), config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: path},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = db.DB().Exec(`INSERT INTO users (id, username, password_hash) VALUES
		(1, 'fixture-one', 'not-a-credential'), (2, 'fixture-two', 'not-a-credential') ON CONFLICT DO NOTHING`)
	if err != nil {
		t.Fatal(err)
	}
	return db, NewStore(db.DB())
}

type catalogFixture struct {
	mu       sync.Mutex
	calls    map[string]int
	failures map[string]int
}

func (f *catalogFixture) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[path]
}

func (f *catalogFixture) fail(path string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[path] = status
}

func candidate(id int, title, date string) tmdb.SearchResult {
	return tmdb.SearchResult{ID: id, Title: title, Name: title, ReleaseDate: date, FirstAirDate: date,
		PosterPath: "/poster.jpg", VoteAverage: 7, Popularity: 10, Overview: "Fixture overview"}
}

func (f *catalogFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls[r.URL.Path]++
	status := f.failures[r.URL.Path]
	f.mu.Unlock()
	if status != 0 {
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`invalid JSON`))
		} else {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"status_message":"provider secret api_key=never-echo"}`))
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	encode := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	credits := tmdb.CreditsResponse{
		Cast: []tmdb.CastEntry{{ID: 10, Name: "Repeated Actor", Order: 0}, {ID: 10, Name: "Repeated Actor", Order: 1}},
		Crew: []tmdb.CrewEntry{{ID: 20, Name: "Repeated Director", Job: "Director"}},
	}
	genres := []tmdb.GenreResponse{{ID: 35, Name: "Comedy"}, {ID: 10749, Name: "Romance"}}
	switch {
	case r.URL.Path == "/collection/50":
		adult := candidate(108, "Adult collection part", "2020-01-01")
		adult.Adult = true
		encode(tmdb.CollectionResponse{ID: 50, Name: "Official Movie Set", Overview: "TMDB set description", PosterPath: "/set.jpg",
			Parts: []tmdb.SearchResult{
				candidate(106, "Upcoming part", "2030-01-01"),
				candidate(1, "Known with file", "2010-01-01"),
				candidate(2, "Known without file", "2011-01-01"),
				candidate(101, "Missing part", "2020-01-01"),
				candidate(101, "Duplicate", "2020-01-01"),
				candidate(107, "Undated part", ""),
				adult,
				candidate(5, "Previously deleted", "2019-01-01"),
			}})
	case strings.HasPrefix(r.URL.Path, "/collection/"):
		http.NotFound(w, r)
	case r.URL.Path == "/person/10/combined_credits":
		cast := []tmdb.CombinedCreditEntry{}
		for _, c := range []tmdb.SearchResult{
			candidate(101, "Missing part", "2020-01-01"), candidate(102, "Actor pick", "2020-02-01"),
			candidate(103, "Adult pick", "2020-01-01"), candidate(104, "Future pick", "2031-01-01"),
			candidate(105, "Unknown date pick", ""), candidate(3, "Known monitored", "2020-01-01"),
		} {
			cast = append(cast, tmdb.CombinedCreditEntry{ID: c.ID, MediaType: "movie", Title: c.Title,
				ReleaseDate: c.ReleaseDate, VoteAverage: c.VoteAverage, Adult: c.ID == 103})
		}
		cast = append(cast, tmdb.CombinedCreditEntry{ID: 201, MediaType: "tv", Name: "Actor series", FirstAirDate: "2020-01-01"})
		encode(tmdb.CombinedCreditsResponse{Cast: cast})
	case r.URL.Path == "/person/20/combined_credits":
		encode(tmdb.CombinedCreditsResponse{Crew: []tmdb.CombinedCreditEntry{
			{ID: 109, MediaType: "movie", Title: "Director pick", ReleaseDate: "2020-01-01", Job: "Director"},
			{ID: 110, MediaType: "movie", Title: "Writer only", ReleaseDate: "2020-01-01", Job: "Writer"},
			{ID: 209, MediaType: "tv", Name: "Director series", FirstAirDate: "2020-01-01", Job: "Director"},
		}})
	case strings.HasSuffix(r.URL.Path, "/recommendations"):
		if strings.HasPrefix(r.URL.Path, "/tv/") {
			encode(tmdb.SearchResponse{Results: []tmdb.SearchResult{candidate(200, "Related series", "2020-01-01"),
				candidate(1, "Known series", "2010-01-01")}})
		} else {
			encode(tmdb.SearchResponse{Results: []tmdb.SearchResult{candidate(101, "Missing part", "2020-01-01"),
				candidate(3, "Known monitored", "2010-01-01"), candidate(4, "Known unmonitored", "2010-01-01")}})
		}
	case r.URL.Path == "/discover/movie":
		if r.URL.Query().Get("with_genres") == "35,10749" {
			encode(tmdb.SearchResponse{Results: []tmdb.SearchResult{candidate(111, "Rom-com pick", "2020-01-01")}})
		} else {
			encode(tmdb.SearchResponse{Results: []tmdb.SearchResult{candidate(101, "Missing part", "2020-01-01"),
				candidate(112, "Genre pick", "2021-01-01")}})
		}
	case r.URL.Path == "/discover/tv":
		encode(tmdb.SearchResponse{Results: []tmdb.SearchResult{candidate(202, "Genre series", "2020-01-01")}})
	case strings.HasPrefix(r.URL.Path, "/movie/"):
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/movie/"))
		m := tmdb.MovieResponse{ID: id}
		if id == 1 || id == 2 {
			m.Collection = &tmdb.CollectionReference{ID: 50}
			m.Credits, m.Genres = credits, genres
		}
		encode(m)
	case strings.HasPrefix(r.URL.Path, "/tv/"):
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/tv/"))
		encode(tmdb.TVResponse{ID: id, Credits: credits, Genres: []tmdb.GenreResponse{{ID: 35, Name: "Comedy"}}})
	case r.URL.Path == "/person/10":
		encode(tmdb.PersonResponse{ID: 10, Name: "Repeated Actor"})
	default:
		http.NotFound(w, r)
	}
}

func fixtureService(t *testing.T) (*Service, *catalogFixture, *movieLibrary, *seriesLibrary) {
	t.Helper()
	_, store := openStore(t, filepath.Join(t.TempDir(), "discovery.db"))
	fixture := &catalogFixture{calls: make(map[string]int), failures: make(map[string]int)}
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)
	deleted := libraryMovie(5)
	now := time.Now()
	deleted.DeletedAt = &now
	library := &movieLibrary{items: []*movies.Movie{libraryMovie(2), libraryMovie(1), libraryMovie(3), libraryMovie(4), deleted},
		files: map[string][]*movies.MovieFile{"movie-1": {{ID: "file-1"}}}}
	library.items[3].MonitoringStatus = movies.MonitoringStatusUnmonitored
	tv := &seriesLibrary{items: []*series.Series{
		{ID: "series-1", Title: "Library TV 1", TMDBID: pointer("1")},
		{ID: "series-2", Title: "Library TV 2", TMDBID: pointer("2")},
	}}
	service := NewService(tmdb.NewClient(tmdb.Config{APIKey: "fixture-key", BaseURL: server.URL}), library, tv, store)
	service.now = func() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }
	return service, fixture, library, tv
}

func recommendationIDs(items []Recommendation) []int {
	out := []int{}
	for _, item := range items {
		out = append(out, item.TMDBID)
	}
	return out
}

func TestRecommendationsSourcesFilteringAndLiveOverlay(t *testing.T) {
	service, fixture, library, _ := fixtureService(t)
	ctx := context.Background()
	first, err := service.Recommendations(ctx, 1, "movie", false)
	if err != nil {
		t.Fatal(err)
	}
	ids := recommendationIDs(first.Items)
	want := []int{101, 112, 5, 109, 102, 111}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("recommendation ranking/filtering: got %v want %v", ids, want)
	}
	if first.Items[0].Source != "collection" || first.Items[0].CollectionID != 50 || len(first.Items[0].Reasons) < 4 {
		t.Fatalf("missing real merged provenance: %+v", first.Items[0])
	}
	if first.Stale || len(first.Warnings) != 0 || first.GeneratedAt.IsZero() {
		t.Fatalf("incorrect status: %+v", first.ResultStatus)
	}
	for _, path := range []string{"/collection/50", "/person/10/combined_credits", "/person/20/combined_credits", "/discover/movie", "/movie/1/recommendations"} {
		if fixture.count(path) == 0 {
			t.Errorf("source never requested: %s", path)
		}
	}
	second, err := service.Recommendations(ctx, 1, "movie", false)
	if err != nil || !reflect.DeepEqual(first, second) || fixture.count("/person/10/combined_credits") != 1 {
		t.Fatalf("non-deterministic or ineffective cache: %v, %v", second, err)
	}
	upcoming, err := service.Recommendations(ctx, 1, "movie", true)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, item := range upcoming.Items {
		seen[item.TMDBID] = true
	}
	if !seen[104] || !seen[106] || seen[105] || seen[107] || seen[108] || seen[103] {
		t.Fatalf("incorrect upcoming/adult/unknown-date handling: %v", seen)
	}
	// Catalog cache stays warm, but newly added and newly dismissed titles must
	// immediately disappear. No file is required for library membership.
	library.items = append(library.items, libraryMovie(101))
	if err := service.store.Dismiss(ctx, 1, "movie", 102); err != nil {
		t.Fatal(err)
	}
	after, err := service.Recommendations(ctx, 1, "movie", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range after.Items {
		if item.TMDBID == 101 || item.TMDBID == 102 {
			t.Fatalf("stale library/dismissal overlay: %+v", item)
		}
	}
	otherUser, err := service.Recommendations(ctx, 2, "movie", false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range otherUser.Items {
		found = found || item.TMDBID == 102
	}
	if !found {
		t.Fatal("one user's dismissal leaked to another user")
	}
	if library.fileGets != 0 {
		t.Fatal("recommendations must not equate library membership with file presence")
	}
}

func TestSeriesRecommendationsAllSources(t *testing.T) {
	service, fixture, _, _ := fixtureService(t)
	out, err := service.Recommendations(context.Background(), 1, "series", false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := recommendationIDs(out.Items), []int{200, 209, 201, 202}; !reflect.DeepEqual(got, want) {
		t.Fatalf("series recommendations: got %v want %v", got, want)
	}
	for _, item := range out.Items {
		if item.MediaType != "series" || item.Title == "" || item.ReleaseDate == "" {
			t.Fatalf("incorrect TV conversion: %+v", item)
		}
	}
	if fixture.count("/collection/50") != 0 || fixture.count("/tv/1/recommendations") != 1 {
		t.Fatal("series used movie sets or missed TV recommendations")
	}
}

func TestCollectionsMembershipDatesAndFreshFileStatus(t *testing.T) {
	service, _, library, _ := fixtureService(t)
	ctx := context.Background()
	list, err := service.Collections(ctx)
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("collections: %+v, %v", list, err)
	}
	want := Collection{ID: 50, Name: "Official Movie Set", Overview: "TMDB set description",
		PosterPath: posterCDN + "/set.jpg", Total: 6, InLibrary: 2, Missing: 3, Upcoming: 1}
	if list.Items[0] != want || library.fileGets != 0 {
		t.Fatalf("collection summary: %+v", list.Items[0])
	}
	detail, err := service.Collection(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Collection != want || detail.Items[0].TMDBID != 1 || detail.Items[len(detail.Items)-1].TMDBID != 107 {
		t.Fatalf("release order and summary: %+v", detail)
	}
	if !detail.Items[0].InLibrary || detail.Items[0].MediaID != "movie-1" || detail.Items[0].HasFile == nil || !*detail.Items[0].HasFile || !detail.Items[0].Monitored {
		t.Fatalf("known/file status: %+v", detail.Items[0])
	}
	if !detail.Items[1].InLibrary || detail.Items[1].HasFile == nil || *detail.Items[1].HasFile {
		t.Fatalf("monitored without files must still be known: %+v", detail.Items[1])
	}
	library.items = append(library.items, libraryMovie(101))
	library.files["movie-1"] = nil
	library.files["movie-101"] = []*movies.MovieFile{{ID: "deleted-file", DeletedAt: pointer(time.Now())}}
	detail, err = service.Collection(ctx, 50)
	if err != nil || detail.Collection.InLibrary != 3 || detail.Collection.Missing != 2 || *detail.Items[0].HasFile {
		t.Fatalf("cached collection didn't overlay live library/files: %+v, %v", detail, err)
	}
	library.fileErr = errors.New("private database error")
	detail, err = service.Collection(ctx, 50)
	if err != nil || detail.Items[0].HasFile != nil || len(detail.Warnings) == 0 {
		t.Fatalf("unknown file status must be omitted with warning: %+v, %v", detail, err)
	}
}

func TestDismissalsPersistAndIsolateOwnersAndTypes(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "restart.db")
	db, store := openStore(t, path)
	for range 2 {
		if err := store.Dismiss(ctx, 1, "movie", 77); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, newStore := openStore(t, path)
	for _, tc := range []struct {
		user int64
		kind string
		want bool
	}{{1, "movie", true}, {2, "movie", false}, {1, "series", false}} {
		ids, err := newStore.Dismissed(ctx, tc.user, tc.kind)
		if err != nil || ids[77] != tc.want {
			t.Fatalf("persisted dismissal user=%d type=%s: %v, %v", tc.user, tc.kind, ids, err)
		}
	}
	for _, tc := range []struct {
		user int64
		kind string
		id   int
	}{{0, "movie", 1}, {1, "tv", 1}, {1, "movie", 0}, {-1, "series", 2}} {
		if err := newStore.Dismiss(ctx, tc.user, tc.kind, tc.id); err == nil {
			t.Fatal("invalid dismissal accepted")
		}
	}
	if _, err := reopened.DB().Exec(`DELETE FROM users WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := reopened.DB().QueryRow(`SELECT COUNT(*) FROM discovery_dismissals`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("user deletion did not cascade: count=%d, err=%v", count, err)
	}
}

func TestCatalogPartialFailureStaleTTLAndMissingCollection(t *testing.T) {
	service, fixture, _, _ := fixtureService(t)
	ctx := context.Background()
	fixture.fail("/person/10/combined_credits", http.StatusUnauthorized)
	out, err := service.Recommendations(ctx, 1, "movie", false)
	if err != nil || len(out.Items) == 0 || len(out.Warnings) == 0 || out.Stale {
		t.Fatalf("partial failure status: %+v, %v", out, err)
	}
	if strings.Contains(strings.Join(out.Warnings, " "), "never-echo") {
		t.Fatal("provider body leaked into warnings")
	}
	first, err := service.Collection(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	originalTime := service.now()
	service.now = func() time.Time { return originalTime.Add(cacheTTL + time.Minute) }
	fixture.fail("/collection/50", http.StatusOK) // malformed response: immediate transient failure
	stale, err := service.Collection(ctx, 50)
	if err != nil || !stale.Stale || len(stale.Warnings) == 0 || !stale.GeneratedAt.Equal(first.GeneratedAt) {
		t.Fatalf("stale fallback: %+v, %v", stale, err)
	}
	fixture.fail("/collection/50", http.StatusNotFound)
	_, err = service.Collection(ctx, 50)
	var typed *tmdb.ClientError
	if !errors.As(err, &typed) || typed.Code != tmdb.ErrCodeNotFound {
		t.Fatalf("genuine missing collection must not use stale data: %v", err)
	}
	fixture.fail("/collection/50", http.StatusOK)
	service.now = func() time.Time { return originalTime.Add(maxStaleAge + time.Minute) }
	if _, err := service.Collection(ctx, 50); err == nil {
		t.Fatal("over-age stale data must not be used")
	}
}

func TestEmptyLibraryFailureAndCancellation(t *testing.T) {
	service, fixture, library, _ := fixtureService(t)
	ctx := context.Background()
	library.items = nil
	empty, err := service.Recommendations(ctx, 1, "movie", false)
	if err != nil || empty.Items == nil || empty.Warnings == nil || len(empty.Items) != 0 || fixture.count("/discover/movie") != 0 {
		t.Fatalf("empty library should return empty arrays without fabricated sources: %+v, %v", empty, err)
	}
	library.items = []*movies.Movie{libraryMovie(1)}
	fixture.fail("/movie/1", http.StatusUnauthorized)
	if _, err := service.Recommendations(ctx, 1, "movie", false); !errors.Is(err, ErrProvider) {
		t.Fatalf("total provider failure must not look like success: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := service.Recommendations(cancelled, 1, "movie", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	service.client = tmdb.NewClient(tmdb.Config{})
	if _, err := service.Collections(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("missing configuration lost: %v", err)
	}
}

func TestBoundedSeedsExcludeEntirePaginatedLibrary(t *testing.T) {
	service, fixture, library, _ := fixtureService(t)
	library.items = nil
	for id := 1000; id >= 1; id-- {
		library.items = append(library.items, libraryMovie(id))
	}
	library.items = append(library.items, libraryMovie(2000))
	out, err := service.Recommendations(context.Background(), 1, "movie", false)
	if err != nil || len(out.Items) != 0 || len(out.Warnings) == 0 {
		t.Fatalf("full local exclusion/sampling status: %+v, %v", out, err)
	}
	if fixture.count("/movie/17") != 0 || fixture.count("/movie/16") != 1 {
		t.Fatal("external detail fanout exceeded seed budget")
	}
}

func TestRepeatedCreditsNeedDistinctLibraryTitlesAndKeepDirectorSource(t *testing.T) {
	service, fixture, library, _ := fixtureService(t)
	library.items = []*movies.Movie{libraryMovie(1)}
	out, err := service.Recommendations(context.Background(), 1, "movie", false)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.count("/person/10/combined_credits") != 0 || fixture.count("/person/20/combined_credits") != 0 {
		t.Fatal("duplicate credits in one movie incorrectly triggered repeated person")
	}
	for _, item := range out.Items {
		if item.Source == "actor" || item.Source == "director" {
			t.Fatalf("single-title credit provenance fabricated: %+v", item)
		}
	}
	credits := tmdb.CreditsResponse{Crew: []tmdb.CrewEntry{{ID: 99, Name: "Director", Job: "Director"}}}
	for id := range 10 {
		credits.Cast = append(credits.Cast, tmdb.CastEntry{ID: id + 1, Name: "Actor", Order: id})
	}
	people := repeatedPeople([]seedDetail{{credits: credits}, {credits: credits}})
	if len(people) != maxPeople {
		t.Fatalf("expected bounded people budget, got %d", len(people))
	}
	director := false
	for _, person := range people {
		director = director || person.role == "director"
	}
	if !director {
		t.Fatal("recurring actors crowded out the director source")
	}
}

func TestRecommendationLimitAndIDTieBreaking(t *testing.T) {
	service, _, library, _ := fixtureService(t)
	library.items = []*movies.Movie{libraryMovie(1)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/1":
			_ = json.NewEncoder(w).Encode(tmdb.MovieResponse{ID: 1})
		case "/movie/1/recommendations":
			items := []tmdb.SearchResult{}
			for id := 1120; id >= 1001; id-- {
				items = append(items, candidate(id, "Equally ranked", "2020-01-01"))
			}
			_ = json.NewEncoder(w).Encode(tmdb.SearchResponse{Results: items})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service.client = tmdb.NewClient(tmdb.Config{APIKey: "fixture-key", BaseURL: server.URL})
	out, err := service.Recommendations(context.Background(), 1, "movie", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != maxResults {
		t.Fatalf("recommendation cap: count=%d", len(out.Items))
	}
	for index, item := range out.Items {
		if item.TMDBID != 1001+index {
			t.Fatalf("unstable tie break at %d: %+v", index, item)
		}
	}
}

func TestCollectionsNeverInferUniverseMembership(t *testing.T) {
	service, fixture, library, _ := fixtureService(t)
	movie := libraryMovie(3)
	movie.Title = "Disney MCU Avengers"
	library.items = []*movies.Movie{movie}
	out, err := service.Collections(context.Background())
	if err != nil || len(out.Items) != 0 || fixture.count("/collection/50") != 0 {
		t.Fatalf("inferred a collection not identified by TMDB: %+v, %v", out, err)
	}
}
