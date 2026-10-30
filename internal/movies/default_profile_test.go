package movies_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/importlists"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/storage"
)

func profileDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "profiles.db")},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db.DB()
}

func execProfileSQL(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func seedAnyProfiles(t *testing.T, db *sql.DB) {
	t.Helper()
	execProfileSQL(t, db, `INSERT INTO quality_profiles_v2(id,name,cutoff) VALUES('not-any','4K','2160p'),('any-current','Any','480p')`)
	execProfileSQL(t, db, `INSERT INTO quality_profiles(id,name,cutoff) VALUES('any-legacy','Any','480p')`)
}

func TestResolveDefaultAnyProfile(t *testing.T) {
	t.Parallel()
	db := profileDB(t)
	seedAnyProfiles(t, db)
	ctx := context.Background()
	for _, selection := range []string{"", " \t\n", "default"} {
		got, err := movies.ResolveQualityProfileID(ctx, db, selection)
		if err != nil || got != "any-current" {
			t.Fatalf("selection %q: got %q %v, want current Any profile", selection, got, err)
		}
	}
	got, err := movies.ResolveQualityProfileID(ctx, db, "not-any")
	if err != nil || got != "not-any" {
		t.Fatalf("explicit selection overwritten: %q %v", got, err)
	}
	execProfileSQL(t, db, `DELETE FROM quality_profiles_v2 WHERE name='Any'`)
	got, err = movies.ResolveQualityProfileID(ctx, db, "")
	if err != nil || got != "any-legacy" {
		t.Fatalf("legacy Any fallback: %q %v", got, err)
	}
	execProfileSQL(t, db, `UPDATE quality_profiles SET deleted_at=CURRENT_TIMESTAMP WHERE id='any-legacy'`)
	if _, err := movies.ResolveQualityProfileID(ctx, db, ""); err == nil {
		t.Fatal("soft-deleted Any profile must not be used")
	}
}

func TestDefaultProfileLookupErrorsAreNotHiddenByLegacyFallback(t *testing.T) {
	t.Parallel()
	db := profileDB(t)
	seedAnyProfiles(t, db)
	execProfileSQL(t, db, `ALTER TABLE quality_profiles_v2 RENAME TO unavailable_quality_profiles_v2`)
	if _, err := movies.ResolveQualityProfileID(context.Background(), db, ""); err == nil {
		t.Fatal("current profile lookup failure silently fell back to a legacy profile")
	}
}

func TestNewMovieAndSeriesPersistDefaultButLeaveExistingTitlesAlone(t *testing.T) {
	t.Parallel()
	db := profileDB(t)
	seedAnyProfiles(t, db)
	ctx := context.Background()
	for _, selection := range []string{"", "  ", "default", "not-any"} {
		id := "title-" + selection
		want := selection
		if strings.TrimSpace(selection) == "" || selection == "default" {
			want = "any-current"
		}
		movie := &movies.Movie{ID: id, Title: "Title", LibraryID: "lib", QualityProfileID: selection}
		if err := movies.NewService(movies.NewRepository(db)).AddMovie(ctx, movie); err != nil {
			t.Fatal(err)
		}
		show := &series.Series{ID: id, Title: "Title", QualityProfileID: selection, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := series.NewRepository(db).CreateSeries(ctx, show); err != nil {
			t.Fatal(err)
		}
		if movie.QualityProfileID != want || show.QualityProfileID != want {
			t.Fatalf("response models did not receive resolved profile: movie=%q show=%q", movie.QualityProfileID, show.QualityProfileID)
		}
		for _, table := range []string{"movies", "series"} {
			var profile string
			if err := db.QueryRowContext(ctx, `SELECT quality_profile_id FROM `+table+` WHERE id=?`, id).Scan(&profile); err != nil {
				t.Fatal(err)
			}
			if profile != want {
				t.Fatalf("%s persisted %q, want %q", table, profile, want)
			}
		}
	}
	execProfileSQL(t, db, `INSERT INTO movies(id,title,quality_profile_id) VALUES('existing','Existing','')`)
	execProfileSQL(t, db, `INSERT INTO series(id,title,year,quality_profile_id) VALUES('existing','Existing',2026,'')`)
	for _, table := range []string{"movies", "series"} {
		var profile string
		if err := db.QueryRowContext(ctx, `SELECT quality_profile_id FROM `+table+` WHERE id='existing'`).Scan(&profile); err != nil {
			t.Fatal(err)
		}
		if profile != "" {
			t.Fatalf("existing %s profile silently changed: %q", table, profile)
		}
	}
}

func TestRealDefaultProfileIDIsPreserved(t *testing.T) {
	t.Parallel()
	for _, table := range []string{"quality_profiles_v2", "quality_profiles"} {
		t.Run(table, func(t *testing.T) {
			db := profileDB(t)
			seedAnyProfiles(t, db)
			execProfileSQL(t, db, `INSERT INTO `+table+`(id,name,cutoff) VALUES('default','Explicit profile','480p')`)
			got, err := movies.ResolveQualityProfileID(context.Background(), db, "default")
			if err != nil || got != "default" {
				t.Fatalf("real default ID must be preserved: got %q err=%v", got, err)
			}
		})
	}
}

func TestMissingQualityProfileIncludesDanglingIDsButNotValidLegacyIDs(t *testing.T) {
	t.Parallel()
	db := profileDB(t)
	seedAnyProfiles(t, db)
	execProfileSQL(t, db, `INSERT INTO quality_profiles(id,name,cutoff,deleted_at) VALUES('deleted','Old','480p',CURRENT_TIMESTAMP)`)
	ctx := context.Background()
	movieRepo, seriesRepo := movies.NewRepository(db), series.NewRepository(db)
	for _, profile := range []string{"", "  ", "default", "dangling", "deleted", "any-current", "any-legacy"} {
		// Seed invalid historical selections without passing through new-title defaulting.
		movie := &movies.Movie{ID: profile, Title: "Title", LibraryID: "lib", QualityProfileID: "any-current"}
		if err := movies.NewService(movieRepo).AddMovie(ctx, movie); err != nil {
			t.Fatal(err)
		}
		show := &series.Series{ID: movie.ID, Title: "Title", QualityProfileID: "any-current", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := seriesRepo.CreateSeries(ctx, show); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"movies", "series"} {
			execProfileSQL(t, db, `UPDATE `+table+` SET quality_profile_id=? WHERE id=?`, profile, movie.ID)
		}
		storedMovie, err := movieRepo.GetMovie(ctx, movie.ID)
		if err != nil {
			t.Fatal(err)
		}
		storedShow, err := seriesRepo.GetSeries(ctx, show.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := profile != "any-current" && profile != "any-legacy"
		if storedMovie.QualityProfileMissing != want || storedShow.QualityProfileMissing != want {
			t.Fatalf("profile %q: movie missing=%v show missing=%v want=%v", profile, storedMovie.QualityProfileMissing, storedShow.QualityProfileMissing, want)
		}
		if storedMovie.QualityProfileID != profile || storedShow.QualityProfileID != profile {
			t.Fatal("reading existing titles changed their profile")
		}
		storedMovie.QualityProfileID = "any-current"
		storedShow.QualityProfileID = "any-legacy"
		if err := movieRepo.UpdateMovie(ctx, storedMovie); err != nil {
			t.Fatal(err)
		}
		if err := seriesRepo.UpdateSeries(ctx, storedShow); err != nil {
			t.Fatal(err)
		}
		if storedMovie.QualityProfileMissing || storedShow.QualityProfileMissing {
			t.Fatal("saving a valid profile did not clear the response warning")
		}
	}
}

func TestDiscoverImportListDefaultsToAny(t *testing.T) {
	t.Parallel()
	db := profileDB(t)
	seedAnyProfiles(t, db)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := importlists.NewStore(db)
	manager := importlists.NewSyncManager(store, logger)
	manager.SetMoviesService(movies.NewService(movies.NewRepository(db)))
	router := importlists.Router(store, manager, logger)
	for _, selection := range []string{"", "default", "not-any"} {
		body := `{"name":"Discover ` + selection + `","list_type":"tmdb_popular","mode":"discover","library_path":"lib","quality_profile_id":"` + selection + `"}`
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create list: %d %s", rec.Code, rec.Body.String())
		}
		var list importlists.ImportList
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		if list.QualityProfileID != selection {
			t.Fatalf("list inserted an unexpected profile: %q", list.QualityProfileID)
		}
		item := &importlists.ImportListItem{ID: "item-" + list.ID, ListID: list.ID, Title: "Discover " + list.ID, Status: importlists.ItemStatusPending}
		if err := store.UpsertItem(context.Background(), item); err != nil {
			t.Fatal(err)
		}
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/discover/add", strings.NewReader(`{"item_id":"`+item.ID+`"}`)))
		if rec.Code != http.StatusOK {
			t.Fatalf("discover add: %d %s", rec.Code, rec.Body.String())
		}
		want := "any-current"
		if selection == "not-any" {
			want = selection
		}
		var got string
		if err := db.QueryRowContext(context.Background(), `SELECT quality_profile_id FROM movies WHERE title=?`, item.Title).Scan(&got); err != nil || got != want {
			t.Fatalf("discover selection %q: got %q err=%v want=%q", selection, got, err, want)
		}
	}
}

func TestProfileRepairClearsNativeAPIWarnings(t *testing.T) {
	t.Parallel()
	db := profileDB(t)
	seedAnyProfiles(t, db)
	ctx := context.Background()
	movieRepo, seriesRepo := movies.NewRepository(db), series.NewRepository(db)
	if err := movies.NewService(movieRepo).AddMovie(ctx, &movies.Movie{ID: "repair", Title: "Repair", LibraryID: "lib"}); err != nil {
		t.Fatal(err)
	}
	if err := seriesRepo.CreateSeries(ctx, &series.Series{ID: "repair", Title: "Repair", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"movies", "series"} {
		execProfileSQL(t, db, `UPDATE `+table+` SET quality_profile_id='default' WHERE id='repair'`)
	}
	for _, tc := range []struct {
		name   string
		router http.Handler
		body   string
	}{
		{"movie", movies.Router(movies.NewService(movieRepo)), `{"quality_profile_id":"any-current"}`},
		{"series", series.Router(series.NewService(seriesRepo, "")), `{"qualityProfileId":"any-current"}`},
	} {
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			rec := httptest.NewRecorder()
			tc.router.ServeHTTP(rec, httptest.NewRequest(method, "/repair", strings.NewReader(tc.body)))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s: %d %s", method, tc.name, rec.Code, rec.Body.String())
			}
			var response struct {
				QualityProfileID      string `json:"qualityProfileId"`
				QualityProfileMissing *bool  `json:"qualityProfileMissing"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			wantMissing := method == http.MethodGet
			if response.QualityProfileMissing == nil || *response.QualityProfileMissing != wantMissing {
				t.Fatalf("%s %s: warning not updated: %s", method, tc.name, rec.Body.String())
			}
			if !wantMissing && response.QualityProfileID != "any-current" {
				t.Fatalf("repaired profile not returned: %s", rec.Body.String())
			}
		}
	}
}

func TestNoAnyProfileFailsBeforeCreatingUnsearchableTitles(t *testing.T) {
	t.Parallel()
	db := profileDB(t)
	ctx := context.Background()
	execProfileSQL(t, db, `INSERT INTO quality_profiles_v2(id,name,cutoff) VALUES('not-any','4K','2160p')`)
	if err := movies.NewRepository(db).AddMovie(ctx, &movies.Movie{ID: "movie", Title: "Movie"}); err == nil {
		t.Fatal("movie created without a default profile")
	}
	if err := series.NewRepository(db).CreateSeries(ctx, &series.Series{ID: "show", Title: "Show"}); err == nil {
		t.Fatal("show created without a default profile")
	}
	for _, table := range []string{"movies", "series"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s rows=%d err=%v", table, count, err)
		}
	}
}

func TestDiscoveryMovieAddReturnsActualAnyProfile(t *testing.T) {
	t.Parallel()
	db := profileDB(t)
	seedAnyProfiles(t, db)
	router := movies.Router(movies.NewService(movies.NewRepository(db)))
	for i, body := range []string{
		`{"title":"No selection","library_id":"lib"}`,
		`{"title":"Explicit selection","library_id":"lib","quality_profile_id":"not-any"}`,
		`{"title":"Whitespace selection","library_id":"lib","quality_profile_id":"  "}`,
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("movie add status=%d body=%s", rec.Code, rec.Body)
		}
		var response struct {
			QualityProfileID string `json:"qualityProfileId"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		want := "any-current"
		if i == 1 {
			want = "not-any"
		}
		if response.QualityProfileID != want {
			t.Fatalf("add returned %q, want %q", response.QualityProfileID, want)
		}
	}
}

type profileMetadata struct{}

func (profileMetadata) FindMovieByQuery(context.Context, string, int) ([]*metadata.MovieMetadata, error) {
	return nil, nil
}

func (profileMetadata) FindMovieByTMDBID(context.Context, string) (*metadata.MovieMetadata, error) {
	return &metadata.MovieMetadata{Title: "TMDB Movie", Year: 2026}, nil
}

func TestTMDBMovieAddUsesAnyAndRemainsIdempotent(t *testing.T) {
	t.Parallel()
	db := profileDB(t)
	seedAnyProfiles(t, db)
	svc := movies.NewService(movies.NewRepository(db), movies.WithMetadata(profileMetadata{}))
	got, err := svc.AddMovieByTMDBID(context.Background(), "123", "", "lib", true)
	if err != nil || got.QualityProfileID != "any-current" {
		t.Fatalf("TMDB add: %+v %v", got, err)
	}
	got, err = svc.AddMovieByTMDBID(context.Background(), "123", "not-any", "lib", true)
	if err != nil || got.QualityProfileID != "any-current" {
		t.Fatalf("existing title changed: %+v %v", got, err)
	}
}
