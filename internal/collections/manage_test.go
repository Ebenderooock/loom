package collections

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/metadata/tmdb"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

type fakeActions struct {
	mu       sync.Mutex
	added    []AddRequest
	searched []string
	profiles map[string]string
	failAdd  string
	failSet  string
	block    chan struct{}
}

func (*fakeActions) QualityProfiles(context.Context) ([]Option, error) {
	return []Option{{ID: "hd", Name: "HD"}, {ID: "uhd", Name: "4K"}}, nil
}

func (*fakeActions) Libraries(_ context.Context, mediaType string) ([]Option, error) {
	return []Option{{ID: mediaType + "-lib", Name: mediaType, QualityProfileID: "hd"}}, nil
}

func (f *fakeActions) SetQualityProfile(_ context.Context, mediaType, mediaID, profile string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if mediaID == f.failSet {
		return errors.New("update failed")
	}
	if f.profiles == nil {
		f.profiles = map[string]string{}
	}
	f.profiles[mediaType+":"+mediaID] = profile
	return nil
}

func (f *fakeActions) Add(_ context.Context, req AddRequest) (string, error) {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if req.TMDBID == f.failAdd {
		return "", errors.New("provider unavailable")
	}
	f.added = append(f.added, req)
	return "new-" + req.TMDBID, nil
}

func (f *fakeActions) SearchMissing(_ context.Context, mediaType, mediaID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searched = append(f.searched, mediaType+":"+mediaID)
	if mediaID == "show-fail" {
		return errors.New("indexers unavailable")
	}
	return nil
}

type statsShows struct {
	items   []*series.Series
	missing map[string]int
}

func (l statsShows) ListSeries(context.Context) ([]*series.Series, error) { return l.items, nil }
func (l statsShows) GetEpisodeStats(_ context.Context, id string) (*series.EpisodeStats, error) {
	return &series.EpisodeStats{MissingEpisodes: l.missing[id]}, nil
}

type fileMovies struct {
	movieLibrary
	files map[string]bool
}

func (l fileMovies) ListMovieFiles(_ context.Context, id string) ([]*movies.MovieFile, error) {
	if l.files[id] {
		return []*movies.MovieFile{{ID: "f"}}, nil
	}
	return nil, nil
}

func ptr(s string) *string { return &s }

// fixtureService builds a reproducible multi-collection library: two
// overlapping collections plus movies/shows covering every eligibility state.
func fixtureService(t *testing.T) (*Service, *fakeActions, *Collection, *Collection) {
	t.Helper()
	store := testStore(t)
	movieItems := []*movies.Movie{
		{ID: "m-owned", Title: "Owned", TMDBID: ptr("1"), QualityProfileID: "hd", MonitoringStatus: "monitored", LibraryID: "movie-lib"},
		{ID: "m-wanted", Title: "Wanted", TMDBID: ptr("2"), QualityProfileID: "hd", MonitoringStatus: "monitored", LibraryID: "movie-lib"},
		{ID: "m-quiet", Title: "Quiet", TMDBID: ptr("3"), QualityProfileID: "uhd", MonitoringStatus: "unmonitored", LibraryID: "movie-lib"},
	}
	shows := statsShows{items: []*series.Series{
		{ID: "show-gap", Title: "Gap", TMDBID: ptr("10"), QualityProfileID: "hd", MonitoringStatus: "all"},
		{ID: "show-full", Title: "Full", TMDBID: ptr("11"), QualityProfileID: "hd", MonitoringStatus: "all"},
	}, missing: map[string]int{"show-gap": 3}}
	actions := &fakeActions{}
	s := NewService(store, nil, nil, fileMovies{movieLibrary{movieItems}, map[string]bool{"m-owned": true}}, shows, nil)
	s.SetActions(actions)
	future := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
	first := saved(t, store, &Collection{Name: "Saga", Members: []Member{
		title("movie", "1"), title("movie", "2"), title("movie", "3"),
		title("movie", "4"), // missing, released
		{ID: "movie-5", MediaType: "movie", Title: "Sequel", ReleaseDate: future, ExternalIDs: map[string]string{"tmdb": "5"}, Sources: []string{}},
		{ID: "movie-imdb", MediaType: "movie", Title: "Unmapped", ExternalIDs: map[string]string{"imdb": "tt9"}, Sources: []string{}},
		title("series", "10"), title("series", "11"), title("series", "12"),
	}})
	second := saved(t, store, &Collection{Name: "Overlap", QualityProfileID: "hd", Members: []Member{title("movie", "2")}})
	return s, actions, first, second
}

func byMember(items []BulkItem) map[string]BulkItem {
	out := map[string]BulkItem{}
	for _, it := range items {
		out[it.MemberID] = it
	}
	return out
}

func waitJob(t *testing.T, s *Service, c *Collection, job *Job) *Job {
	t.Helper()
	for range 200 {
		got, err := s.Job(c.ID, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == StatusDone {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return nil
}

func TestBulkAddPreviewNeverAddsAndExecutesOnlyEligible(t *testing.T) {
	ctx := context.Background()
	s, actions, c, _ := fixtureService(t)
	plan, err := s.PlanAdd(ctx, c.ID, addBody{selection: selection{All: true}})
	if err != nil {
		t.Fatal(err)
	}
	items := byMember(plan.Items)
	want := map[string]string{"movie-1": "in_library", "movie-4": "", "movie-5": "unreleased", "movie-imdb": "unresolved", "series-12": ""}
	for id, reason := range want {
		if items[id].Reason != reason {
			t.Fatalf("%s reason %q, want %q (%+v)", id, items[id].Reason, reason, items[id])
		}
	}
	if items["movie-4"].Status != StatusEligible || items["movie-4"].NewProfileID != "hd" || plan.Settings["movie"].LibraryID != "movie-lib" {
		t.Fatalf("library default settings not resolved: %+v %+v", items["movie-4"], plan.Settings)
	}
	if len(actions.added) != 0 {
		t.Fatal("preview added titles")
	}
	// Collection default overrides the library default for new additions only.
	c.QualityProfileID = "uhd"
	if err := s.store.Save(ctx, c, false); err != nil {
		t.Fatal(err)
	}
	plan, err = s.PlanAdd(ctx, c.ID, addBody{selection: selection{MemberIDs: []string{"movie-4", "movie-1", "series-12"}}, Execute: true, Series: AddSettings{Search: true}})
	if err != nil || plan.Job == nil {
		t.Fatal(err)
	}
	job := waitJob(t, s, c, plan.Job)
	if job.Counts[StatusDone] != 2 || len(actions.added) != 2 {
		t.Fatalf("expected exactly the eligible titles to be added: %+v %+v", job, actions.added)
	}
	for _, req := range actions.added {
		if req.QualityProfileID != "uhd" || !req.Monitored || req.Search != (req.MediaType == "series") {
			t.Fatalf("shared settings not passed: %+v", req)
		}
	}
	if p := actions.profiles; len(p) != 0 {
		t.Fatalf("collection default changed existing titles: %v", p)
	}
	if _, err := s.PlanAdd(ctx, c.ID, addBody{selection: selection{MemberIDs: []string{"movie-1"}}, Execute: true}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nothing eligible must fail explicitly: %v", err)
	}
	if _, err := s.PlanAdd(ctx, c.ID, addBody{selection: selection{All: true, MemberIDs: []string{"movie-1"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("ambiguous selection accepted")
	}
	if _, err := s.PlanAdd(ctx, c.ID, addBody{selection: selection{All: true}, Movie: AddSettings{LibraryID: "other"}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown library accepted")
	}
}

func TestBulkAddPartialFailureAndDedupe(t *testing.T) {
	ctx := context.Background()
	s, actions, c, _ := fixtureService(t)
	actions.failAdd, actions.block = "12", make(chan struct{})
	plan, err := s.PlanAdd(ctx, c.ID, addBody{selection: selection{MemberIDs: []string{"movie-4", "series-12"}}, Execute: true})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.PlanAdd(ctx, c.ID, addBody{selection: selection{MemberIDs: []string{"movie-4"}}})
	if err != nil || byMember(again.Items)["movie-4"].Reason != "already_running" {
		t.Fatalf("concurrent add not deduplicated: %+v %v", again, err)
	}
	close(actions.block)
	job := waitJob(t, s, c, plan.Job)
	items := byMember(job.Items)
	if items["movie-4"].Status != StatusDone || items["movie-4"].MediaID != "new-4" || items["series-12"].Status != StatusFailed || items["series-12"].Detail == "" {
		t.Fatalf("partial failure not explicit: %+v", job.Items)
	}
	if jobs := s.Jobs(c.ID); len(jobs) != 1 || jobs[0].ID != job.ID {
		t.Fatal("job history missing")
	}
	if _, err := s.Job("other", job.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("job leaked across collections")
	}
}

func TestBulkSearchEligibilityNeverAdds(t *testing.T) {
	ctx := context.Background()
	s, actions, c, _ := fixtureService(t)
	plan, err := s.PlanSearch(ctx, c.ID, searchBody{selection: selection{All: true}})
	if err != nil {
		t.Fatal(err)
	}
	items := byMember(plan.Items)
	want := map[string]string{"movie-1": "has_file", "movie-2": "", "movie-3": "unmonitored", "movie-4": "not_in_library", "series-10": "", "series-11": "nothing_missing", "series-12": "not_in_library"}
	for id, reason := range want {
		if items[id].Reason != reason {
			t.Fatalf("%s reason %q, want %q", id, items[id].Reason, reason)
		}
	}
	if items["series-10"].MissingEpisodes != 3 || plan.Counts[StatusEligible] != 2 {
		t.Fatalf("eligibility counts: %+v", plan.Counts)
	}
	plan, err = s.PlanSearch(ctx, c.ID, searchBody{selection: selection{All: true}, Execute: true})
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, s, c, plan.Job)
	if len(actions.added) != 0 || strings.Join(actions.searched, ",") != "movie:m-wanted,series:show-gap" {
		t.Fatalf("search must only search eligible titles and never add: %v %v", actions.searched, actions.added)
	}
}

func TestQualityDefaultIsExplicitAndScopedToSelection(t *testing.T) {
	ctx := context.Background()
	s, actions, c, overlap := fixtureService(t)
	if _, err := s.PlanQuality(ctx, c.ID, qualityBody{selection: selection{All: true}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("apply without a default/profile accepted")
	}
	preview, err := s.PlanQuality(ctx, c.ID, qualityBody{selection: selection{All: true}, QualityProfileID: "uhd"})
	if err != nil {
		t.Fatal(err)
	}
	items := byMember(preview.Items)
	if items["movie-2"].CurrentProfileID != "hd" || items["movie-2"].Status != StatusEligible || items["movie-3"].Reason != "unchanged" || items["movie-4"].Reason != "not_in_library" || len(actions.profiles) != 0 {
		t.Fatalf("quality preview: %+v", preview.Items)
	}
	actions.failSet = "show-full"
	applied, err := s.PlanQuality(ctx, c.ID, qualityBody{selection: selection{MemberIDs: []string{"movie-2", "series-11"}}, QualityProfileID: "uhd", Execute: true})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Counts[StatusDone] != 1 || applied.Counts[StatusFailed] != 1 || len(actions.profiles) != 1 || actions.profiles["movie:m-wanted"] != "uhd" || !strings.Contains(applied.Note, "other collections") {
		t.Fatalf("explicit apply not scoped: %+v %v", applied, actions.profiles)
	}
	got, err := s.store.Get(ctx, overlap.ID)
	if err != nil || got.QualityProfileID != "hd" {
		t.Fatal("overlapping collection default changed")
	}
}

func TestAddMembersFromLibraryAndProviderWithCollisions(t *testing.T) {
	ctx := context.Background()
	s, _, _, _ := fixtureService(t)
	c := saved(t, s.store, &Collection{Name: "Custom", Members: []Member{}})
	got, err := s.AddMembers(ctx, c.ID, nil, []memberInput{
		{MediaType: "movie", MediaID: "m-wanted"},
		{MediaType: "series", Title: "Provider Show", Year: 2020, ExternalIDs: map[string]string{"tmdb": "77"}},
		{MediaType: "movie", MediaID: "m-wanted"},
	})
	if err != nil {
		t.Fatal(err)
	}
	members := map[string]Member{}
	for _, m := range got.Members {
		members[m.MediaType] = m
	}
	if len(got.Members) != 2 || members["movie"].ExternalIDs["tmdb"] != "2" || members["movie"].Title != "Wanted" || !members["movie"].Local || members["series"].Title != "Provider Show" {
		t.Fatalf("members not resolved to real titles: %+v", got.Members)
	}
	stale := c.Revision
	if _, err := s.AddMembers(ctx, c.ID, &stale, []memberInput{{MediaType: "movie", MediaID: "m-owned"}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	if _, err := s.AddMembers(ctx, c.ID, nil, []memberInput{{MediaType: "movie", MediaID: "missing"}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown library title accepted")
	}
	if _, err := s.AddMembers(ctx, c.ID, nil, []memberInput{{MediaType: "movie", Title: "Bad", ExternalIDs: map[string]string{"tmdb": "2", "imdb": "tt1234567"}}}); err != nil {
		t.Fatalf("compatible extra ID rejected: %v", err)
	}
	if _, err := s.AddMembers(ctx, c.ID, nil, []memberInput{{MediaType: "movie", Title: "Clash", ExternalIDs: map[string]string{"tmdb": "99", "imdb": "tt1234567"}}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("colliding provider IDs accepted: %v", err)
	}
	if _, err := s.AddMembers(ctx, c.ID, nil, []memberInput{{MediaType: "movie", Title: "No IDs"}}); err == nil {
		t.Fatal("unresolvable custom title accepted")
	}
}

func TestDeleteKeepsMediaAndSuppressesBackgroundRecreation(t *testing.T) {
	ctx, store := context.Background(), testStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/1":
			_, _ = w.Write([]byte(`{"id":1,"title":"Movie","belongs_to_collection":{"id":10,"name":"Set"}}`))
		case "/collection/10":
			_, _ = w.Write([]byte(`{"id":10,"name":"Set","parts":[{"id":1,"title":"Movie"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	library := movieLibrary{[]*movies.Movie{{ID: "local", TMDBID: ptr("1")}}}
	s := NewService(store, tmdb.NewClient(tmdb.Config{APIKey: "test", BaseURL: server.URL}), nil, library, showLibrary{}, nil)
	member := Member{MediaType: "movie", Title: "Movie", ExternalIDs: map[string]string{"tmdb": "1"}}
	created, err := s.discoverMember(ctx, member)
	if err != nil || len(created) != 1 {
		t.Fatalf("discovery: %v %v", created, err)
	}
	router := Router(s)
	request := func(method, path, role string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r = r.WithContext(auth.WithIdentity(r.Context(), &auth.Identity{UserID: 1, Roles: []string{role}}))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := request("DELETE", fmt.Sprintf("/%s?revision=%d", created[0].ID, created[0].Revision), "user"); w.Code != http.StatusForbidden {
		t.Fatalf("non-admin delete: %d", w.Code)
	}
	if w := request("DELETE", fmt.Sprintf("/%s?revision=%d", created[0].ID, created[0].Revision+5), "admin"); w.Code != http.StatusConflict {
		t.Fatalf("stale delete: %d", w.Code)
	}
	w := request("DELETE", fmt.Sprintf("/%s?revision=%d", created[0].ID, created[0].Revision), "admin")
	var out struct {
		Suppressed int `json:"suppressed_sources"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Suppressed != 1 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if _, err := store.Get(ctx, created[0].ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("collection still exists")
	}
	if items, _ := library.ListMovies(ctx, 10, 0); len(items) != 1 {
		t.Fatal("library media touched")
	}
	again, err := s.discoverMember(ctx, member)
	if err != nil || len(again) != 0 {
		t.Fatalf("background discovery recreated a deleted collection: %v %v", again, err)
	}
	ref := Source{Provider: "tmdb", Kind: "collection", ExternalID: "10"}
	restored, err := s.Import(ctx, ref)
	if err != nil || restored.ID == created[0].ID || len(restored.Members) != 1 {
		t.Fatalf("explicit reimport did not recover: %+v %v", restored, err)
	}
	if suppressed, err := store.Suppressed(ctx, ref); err != nil || suppressed {
		t.Fatal("reimport kept suppression")
	}
}

func TestManagementRoutesRequireAdminAndActions(t *testing.T) {
	s, _, c, _ := fixtureService(t)
	s.actions = nil
	router := Router(s)
	do := func(method, path, body, role string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r = r.WithContext(auth.WithIdentity(r.Context(), &auth.Identity{UserID: 1, Roles: []string{role}}))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	for _, path := range []string{"/bulk/add", "/bulk/search", "/quality"} {
		if got := do("POST", "/"+c.ID+path, `{"all":true}`, "user"); got != http.StatusForbidden {
			t.Fatalf("%s user: %d", path, got)
		}
		if got := do("POST", "/"+c.ID+path, `{"all":true}`, "admin"); got != http.StatusServiceUnavailable {
			t.Fatalf("%s without actions: %d", path, got)
		}
	}
	if got := do("DELETE", "/"+c.ID+"/members/movie-1", "", "user"); got != http.StatusForbidden {
		t.Fatalf("member remove user: %d", got)
	}
	if got := do("GET", "/"+c.ID+"/jobs", "", "user"); got != http.StatusOK {
		t.Fatalf("jobs read: %d", got)
	}
}

func TestRemoveMemberVersusExclude(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	s := &Service{store: store}
	source := title("movie", "1")
	source.Sources = []string{"tmdb:collection:1"}
	both := title("movie", "2")
	both.Sources, both.Local = []string{"tmdb:collection:1"}, true
	local := title("movie", "3")
	local.Local = true
	c := saved(t, store, &Collection{Name: "Mixed", Members: []Member{source, both, local}})
	router := Router(s)
	do := func(member string) int {
		r := httptest.NewRequest("DELETE", "/"+c.ID+"/members/"+member, nil)
		r = r.WithContext(auth.WithIdentity(r.Context(), &auth.Identity{UserID: 1, Roles: []string{"admin"}}))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	if do("movie-1") != http.StatusBadRequest || do("movie-2") != 200 || do("movie-3") != 200 || do("nope") != http.StatusNotFound {
		t.Fatal("member removal semantics")
	}
	got, _ := store.Get(ctx, c.ID)
	if len(got.Members) != 2 || got.Members[1].Local {
		t.Fatalf("remove result: %+v", got.Members)
	}
}
