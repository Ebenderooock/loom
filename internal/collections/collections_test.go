package collections

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := storage.Open(context.Background(), config.StorageConfig{Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "collections.db")}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewStore(db.DB())
}

func title(media, id string) Member {
	return Member{ID: media + "-" + id, MediaType: media, Title: media + " " + id, ExternalIDs: map[string]string{"tmdb": id}, Sources: []string{}}
}

func TestSourceInputLinksAndIDs(t *testing.T) {
	tests := []struct {
		provider, kind, input, want string
	}{
		{"tmdb", "list", " https://www.themoviedb.org/list/8924-movielist?sort=1 ", "8924"},
		{"tmdb", "collection", "https://www.themoviedb.org/collection/10-star-wars", "10"},
		{"tmdb", "list", "https://www.themoviedb.org/list/123-caf%C3%A9", "123"},
		{"tvdb", "list", "https://thetvdb.com/lists/star-wars/", "star-wars"},
		{"tvdb", "list", "https://thetvdb.com/lists/17367", "17367"},
		{"imdb", "list", "https://www.imdb.com/list/ls123456789/?ref_=abc", "ls123456789"},
		{"imdb", "list", "ls123456789", "ls123456789"},
		{"tmdb", "list", "42", "42"},
		{"tmdb", "list", "https://evil.example/list/42", ""},
		{"tmdb", "list", "https://themoviedb.org/collection/42", ""},
		{"tmdb", "list", "https://themoviedb.org.evil.example/list/42", ""},
		{"imdb", "list", "https://imdb.com/list/ls1/extra", ""},
		{"tvdb", "list", "https://thetvdb.com/lists/../../login", ""},
		{"tvdb", "list", "999999999999999999999", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := sourceInput(Source{Provider: tt.provider, Kind: tt.kind, ExternalID: tt.input})
			if tt.want == "" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("expected invalid input, got %#v %v", got, err)
				}
			} else if err != nil || got.ExternalID != tt.want {
				t.Fatalf("got %#v %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestTVDBUnresolvableSlugDoesNotCreateCollection(t *testing.T) {
	store := testStore(t)
	s := NewService(store, nil, nil, nil, nil, nil)
	if _, err := s.Import(context.Background(), Source{Provider: "tvdb", Kind: "list", ExternalID: "https://thetvdb.com/lists/star-wars"}); !errors.Is(err, errSourceLookup) {
		t.Fatalf("expected source lookup error, got %v", err)
	}
	items, err := store.List(context.Background())
	if err != nil || len(items) != 0 {
		t.Fatalf("failed resolution created a collection: %#v %v", items, err)
	}
}

func TestImportLinksUseCanonicalIDs(t *testing.T) {
	ctx := context.Background()
	s := NewService(testStore(t), nil, nil, nil, nil, nil)
	s.fetch = func(_ context.Context, ref Source) (string, []Member, string, error) {
		return ref.Key(), nil, "", nil
	}
	for _, tc := range []struct {
		provider, kind, link, id string
	}{
		{"tmdb", "list", "https://www.themoviedb.org/list/8924-movielist", "8924"},
		{"tmdb", "collection", "https://www.themoviedb.org/collection/10-star-wars", "10"},
		{"imdb", "list", "https://www.imdb.com/list/ls123456789/", "ls123456789"},
	} {
		fromLink, err := s.Import(ctx, Source{Provider: tc.provider, Kind: tc.kind, ExternalID: tc.link})
		if err != nil {
			t.Fatal(err)
		}
		fromID, err := s.Import(ctx, Source{Provider: tc.provider, Kind: tc.kind, ExternalID: tc.id})
		if err != nil || fromID.ID != fromLink.ID || fromLink.Sources[0].ExternalID != tc.id {
			t.Fatalf("link and ID did not resolve to same source: %#v %v", fromID, err)
		}
	}
}

func TestAttachSourceNormalizesBeforeIdentityLookup(t *testing.T) {
	store := testStore(t)
	existing := saved(t, store, &Collection{Name: "Existing", Sources: []Source{{Provider: "tmdb", Kind: "list", ExternalID: "8924"}}})
	other := saved(t, store, &Collection{Name: "Other"})
	s := NewService(store, nil, nil, nil, nil, nil)
	router := Router(s)
	for _, tc := range []struct {
		id     string
		status int
	}{{existing.ID, http.StatusOK}, {other.ID, http.StatusConflict}} {
		req := httptest.NewRequest(http.MethodPost, "/"+tc.id+"/sources", strings.NewReader(`{"provider":"tmdb","kind":"list","external_id":"https://www.themoviedb.org/list/8924-movielist"}`))
		req = req.WithContext(auth.WithIdentity(req.Context(), &auth.Identity{UserID: 1, Roles: []string{"admin"}}))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("attach status %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
		}
	}
}

func TestAttachSourceRejectsProviderMetadata(t *testing.T) {
	store := testStore(t)
	c := saved(t, store, &Collection{Name: "Existing"})
	router := Router(NewService(store, nil, nil, nil, nil, nil))
	for _, field := range []string{`"poster_path":"/source.jpg"`, `"last_sync":"today"`, `"error":"failed"`} {
		t.Run(field, func(t *testing.T) {
			body := `{"provider":"tmdb","kind":"list","external_id":"https://www.themoviedb.org/list/8924",` + field + `}`
			req := httptest.NewRequest(http.MethodPost, "/"+c.ID+"/sources", strings.NewReader(body))
			req = req.WithContext(auth.WithIdentity(req.Context(), &auth.Identity{UserID: 1, Roles: []string{"admin"}}))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("attach status %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func saved(t *testing.T, store *Store, c *Collection) *Collection {
	t.Helper()
	if err := store.Save(context.Background(), c, true); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestMergeIdentityLocalIntentAndRedirects(t *testing.T) {
	ctx, store := context.Background(), testStore(t)
	aRef := Source{Provider: "tmdb", Kind: "collection", ExternalID: "1"}
	bRef := Source{Provider: "imdb", Kind: "list", ExternalID: "ls2"}
	cRef := Source{Provider: "tvdb", Kind: "list", ExternalID: "3"}
	movie := title("movie", "7")
	movie.ExternalIDs["imdb"] = "tt7"
	movie.Local, movie.Title, movie.Sources = true, "Local title", []string{aRef.Key()}
	duplicate := title("movie", "7")
	duplicate.ExternalIDs["imdb"], duplicate.Excluded, duplicate.Sources = "tt7", true, []string{bRef.Key()}
	show := title("series", "7")
	show.Sources = []string{cRef.Key()}
	a := saved(t, store, &Collection{Name: "Marvel", Members: []Member{movie}, Sources: []Source{aRef}})
	b := saved(t, store, &Collection{Name: "Different name", Members: []Member{duplicate, show}, Sources: []Source{bRef, cRef}})
	if err := store.Merge(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	result, err := store.Get(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != a.ID || len(result.Sources) != 3 || len(result.Members) != 2 {
		t.Fatalf("merge result: %#v", result)
	}
	var got Member
	for _, m := range result.Members {
		if m.MediaType == "movie" {
			got = m
		}
	}
	if !got.Local || !got.Excluded || got.Title != "Local title" || len(got.Sources) != 2 {
		t.Fatalf("lost local intent/provenance: %#v", got)
	}
	for _, ref := range []Source{aRef, bRef, cRef} {
		c, err := store.FindSource(ctx, ref)
		if err != nil || c.ID != a.ID {
			t.Fatalf("source not reassociated: %v %#v", err, c)
		}
	}
	third := saved(t, store, &Collection{Name: "Final"})
	if err := store.Merge(ctx, third, result); err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{a.ID, b.ID} {
		c, err := store.Get(ctx, old)
		if err != nil || c.ID != third.ID {
			t.Fatalf("transitive redirect %s: %v %#v", old, err, c)
		}
	}
}

func TestNoNameOrOverlapIdentityAndMultipleMembership(t *testing.T) {
	store, ctx := testStore(t), context.Background()
	for _, ref := range []Source{{Provider: "tmdb", Kind: "collection", ExternalID: "10"}, {Provider: "tmdb", Kind: "list", ExternalID: "10"}} {
		saved(t, store, &Collection{Name: "Same name", Sources: []Source{ref}, Members: []Member{title("movie", "12")}})
	}
	items, err := store.List(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("same-name/overlap collections collapsed: %v %#v", err, items)
	}
}

func TestStaleSaveMergeAndDuplicateSourceAreAtomic(t *testing.T) {
	ctx, store := context.Background(), testStore(t)
	ref := Source{Provider: "tmdb", Kind: "collection", ExternalID: "10"}
	a := saved(t, store, &Collection{Name: "A", Sources: []Source{ref}})
	stale, _ := store.Get(ctx, a.ID)
	a.Name = "Edited"
	if err := store.Save(ctx, a, false); err != nil {
		t.Fatal(err)
	}
	stale.Name = "Overwrite"
	if err := store.Save(ctx, stale, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale save: %v", err)
	}
	duplicate := &Collection{Name: "Duplicate", Sources: []Source{ref}}
	if err := store.Save(ctx, duplicate, true); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate source: %v", err)
	}
	if _, err := store.Get(ctx, duplicate.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("partial create persisted: %v", err)
	}
	b := saved(t, store, &Collection{Name: "B", Members: []Member{title("series", "10")}})
	if err := store.Merge(ctx, b, stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale merge: %v", err)
	}
	if c, err := store.FindSource(ctx, ref); err != nil || c.ID != a.ID {
		t.Fatalf("partial reassociation: %v %#v", err, c)
	}
	result, _ := store.Get(ctx, a.ID)
	if result.Name != "Edited" {
		t.Fatal("stale save overwrote title")
	}
}

func TestRefreshRetainsOverridesExclusionsAndOtherSources(t *testing.T) {
	store, ctx := testStore(t), context.Background()
	ref := Source{Provider: "tmdb", Kind: "collection", ExternalID: "1"}
	other := Source{Provider: "imdb", Kind: "list", ExternalID: "ls2"}
	local := title("movie", "1")
	local.Local, local.Title = true, "My title"
	local.Sources = []string{ref.Key()}
	excluded := title("movie", "2")
	excluded.Excluded = true
	excluded.Sources = []string{ref.Key()}
	removed := title("movie", "3")
	removed.Sources = []string{ref.Key()}
	kept := title("series", "3")
	kept.Sources = []string{other.Key()}
	c := saved(t, store, &Collection{Name: "Mixed", Sources: []Source{ref, other}, Members: []Member{local, excluded, removed, kept}})
	c.Cover = &Cover{Mode: "source", SourceKey: ref.Key()}
	c.Sources[1].PosterPath = "/retained.jpg"
	s := &Service{store: store, fetch: func(_ context.Context, source Source) (string, []Member, string, error) {
		if source.Provider == "imdb" {
			return "", nil, "", fmt.Errorf("private provider error with secret")
		}
		return "Provider title", []Member{title("movie", "1"), title("movie", "2"), title("series", "1")}, "/updated.jpg", nil
	}}
	if err := s.Refresh(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, c, false); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(ctx, c.ID)
	if len(got.Members) != 4 {
		t.Fatalf("membership: %#v", got.Members)
	}
	for _, m := range got.Members {
		if m.MediaType == "movie" && m.ExternalIDs["tmdb"] == "1" && (!m.Local || m.Title != "My title") {
			t.Fatal("lost override")
		}
		if m.MediaType == "movie" && m.ExternalIDs["tmdb"] == "2" && !m.Excluded {
			t.Fatal("lost exclusion")
		}
	}
	if got.Sources[0].LastSync == "" || got.Sources[1].Error == "" || strings.Contains(got.Sources[1].Error, "secret") {
		t.Fatalf("unsafe/hidden source failure: %#v", got.Sources)
	}
	if got.Cover == nil || *got.Cover != *c.Cover || got.Sources[0].PosterPath != "/updated.jpg" || got.Sources[1].PosterPath != "/retained.jpg" {
		t.Fatalf("refresh lost artwork or local cover: %#v", got)
	}
}

func TestConflictingMappingRefreshDoesNotPartiallyApply(t *testing.T) {
	store, ctx := testStore(t), context.Background()
	ref := Source{Provider: "tmdb", Kind: "collection", ExternalID: "1"}
	member := title("movie", "1")
	member.ExternalIDs["imdb"] = "tt1"
	member.Sources = []string{ref.Key()}
	c := saved(t, store, &Collection{Name: "A", Sources: []Source{ref}, Members: []Member{member}})
	s := &Service{fetch: func(context.Context, Source) (string, []Member, string, error) {
		conflict := title("movie", "1")
		conflict.ExternalIDs["imdb"] = "tt2"
		return "", []Member{title("movie", "2"), conflict}, "", nil
	}}
	if err := s.Refresh(ctx, c); err != nil {
		t.Fatal(err)
	}
	if len(c.Members) != 1 || c.Members[0].ExternalIDs["imdb"] != "tt1" || c.Sources[0].Error == "" {
		t.Fatalf("partial refresh: %#v", c)
	}
}

func TestImportIsExactIdempotentAndErrorsAreVisible(t *testing.T) {
	ctx, store := context.Background(), testStore(t)
	calls := 0
	s := &Service{store: store, fetch: func(context.Context, Source) (string, []Member, string, error) {
		calls++
		return "", nil, "", fmt.Errorf("secret source response")
	}}
	ref := Source{Provider: "imdb", Kind: "list", ExternalID: "ls1"}
	first, err := s.Import(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Import(ctx, ref)
	if err != nil || first.ID != second.ID || calls != 1 || first.Sources[0].Error == "" {
		t.Fatalf("import: %#v %#v %v", first, second, err)
	}
}

func TestNamespaceAndBridgeDedupe(t *testing.T) {
	movie, show := title("movie", "42"), title("series", "42")
	imdb := Member{ID: "imdb", MediaType: "movie", Title: "Movie", ExternalIDs: map[string]string{"imdb": "tt42"}, Local: true}
	bridge := title("movie", "42")
	bridge.ExternalIDs["imdb"] = "tt42"
	got, err := coalesce([]Member{movie, show, imdb}, bridge)
	if err != nil || len(got) != 2 {
		t.Fatalf("bridge: %#v %v", got, err)
	}
	for _, m := range got {
		if m.MediaType == "movie" && (!m.Local || len(m.ExternalIDs) != 2) {
			t.Fatal("lost identity or local flag")
		}
	}
}

func TestRouterManagementAuthorizationValidationAndStaleWrites(t *testing.T) {
	store := testStore(t)
	s := &Service{store: store}
	router := Router(s)
	request := func(method, path, body, role string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if role != "" {
			r = r.WithContext(auth.WithIdentity(r.Context(), &auth.Identity{UserID: 1, Roles: []string{role}}))
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/", "/discover", "/a/refresh", "/a/merge", "/a/sources", "/a/members", "/a/bulk/add", "/a/bulk/search", "/a/quality"} {
		if got := request("POST", path, `{}`, "user").Code; got != http.StatusForbidden {
			t.Fatalf("role guard %s: %d", path, got)
		}
		if got := request("POST", path, `{}`, "").Code; got != http.StatusUnauthorized {
			t.Fatalf("auth %s: %d", path, got)
		}
	}
	for _, body := range []string{`{"name":""}`, `{"name":"A","unknown":1}`, `{"name":"A"} {}`, `{"source":{"provider":"imdb","kind":"list","external_id":"https://127.0.0.1/"}}`} {
		if got := request("POST", "/", body, "admin"); got.Code != 400 {
			t.Fatalf("invalid input %s: %d %s", body, got.Code, got.Body)
		}
	}
	c := saved(t, store, &Collection{Name: "A"})
	w := request("PATCH", "/"+c.ID, `{"name":"Edit","overview":"","revision":10}`, "admin")
	if w.Code != 409 {
		t.Fatalf("stale edit: %d %s", w.Code, w.Body)
	}
	w = request("POST", "/"+c.ID+"/members", `{"items":[{"media_type":"movie","title":"A","external_ids":{"tmdb":"1"}}]}`, "admin")
	if w.Code != 200 {
		t.Fatalf("local member: %d %s", w.Code, w.Body)
	}
	got, _ := store.Get(context.Background(), c.ID)
	if len(got.Members) != 1 || !got.Members[0].Local {
		t.Fatal("local member was not persisted")
	}
	destination := saved(t, store, &Collection{Name: "Retained"})
	if err := store.Merge(context.Background(), destination, got); err != nil {
		t.Fatal(err)
	}
	// Revision values can coincide across collections; an old ID must not edit
	// its redirect target just because the supplied revision is now equal.
	w = request("PATCH", "/"+c.ID, fmt.Sprintf(`{"name":"Stale overwrite","overview":"","revision":%d}`, destination.Revision), "admin")
	if w.Code != http.StatusConflict {
		t.Fatalf("redirected stale write: %d %s", w.Code, w.Body)
	}
	retained, _ := store.Get(context.Background(), destination.ID)
	if retained.Name != "Retained" {
		t.Fatal("stale writer edited retained collection")
	}
}

func TestLibraryStateSeparateFromMembership(t *testing.T) {
	c := &Collection{Members: []Member{title("movie", "1"), title("series", "1"), title("movie", "2")}}
	library := []libraryRecord{{member: Member{MediaType: "movie", ExternalIDs: map[string]string{"tmdb": "1"}, InLibrary: true, MediaID: "actual-movie", LibraryID: "library", Monitored: false}, file: func() (*bool, error) { has := false; return &has, nil }}}
	if err := decorate(c, library, true); err != nil {
		t.Fatal(err)
	}
	if c.Total != 3 || c.InLibrary != 1 || c.Members[0].MediaID != "actual-movie" || c.Members[0].Monitored || *c.Members[0].HasFile || c.Members[1].InLibrary {
		t.Fatalf("state conflated: %#v", c)
	}
	body, err := encode(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "actual-movie") {
		t.Fatal("library state persisted as membership")
	}
}
