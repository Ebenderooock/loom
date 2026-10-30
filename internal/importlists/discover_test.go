package importlists

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/metadata/tmdb"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/storage"
)

type discoverMovies struct {
	movies.Service
	existing *movies.Movie
}

func (s *discoverMovies) GetMovieByTMDBID(context.Context, string) (*movies.Movie, error) {
	return s.existing, nil
}
func (s *discoverMovies) GetMovieByIMDBID(context.Context, string) (*movies.Movie, error) {
	return s.existing, nil
}

type discoverSeries struct {
	series.Service
	existing []*series.Series
}

func (s *discoverSeries) ListSeries(context.Context) ([]*series.Series, error) {
	return s.existing, nil
}

func discoverStore(t *testing.T) *Store {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(context.Background(), config.StorageConfig{Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "discover.db")}}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewStore(db.DB())
}

func TestDiscoverDefaultsAndTrackingPreserveSourceWithoutAdding(t *testing.T) {
	ctx := context.Background()
	store := discoverStore(t)
	list := &ImportList{ID: "source", Name: "Watchlist", ListType: ListTypeTMDbList, Enabled: true, Mode: ListModeDiscover, MediaType: MediaTypeMovie, LibraryPath: "/movies", QualityProfileID: "source-profile", MonitorType: MonitorNone, SearchOnAdd: false, APIKey: "must-not-be-exposed"}
	if err := store.Create(ctx, list); err != nil {
		t.Fatal(err)
	}
	item := &ImportListItem{ID: "item", ListID: list.ID, ExternalID: "42", TMDbID: "42", Title: "Title", Status: ItemStatusPending}
	if err := store.UpsertItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	svc := &discoverMovies{}
	manager := NewSyncManager(store, nil)
	manager.SetMoviesService(svc)
	items, err := manager.DiscoverItems(ctx, "movie")
	if err != nil || len(items) != 1 {
		t.Fatalf("feed: %v %+v", err, items)
	}
	if items[0].Defaults == nil || items[0].Defaults.QualityProfileID != "source-profile" || items[0].Defaults.SearchOnAdd || items[0].Defaults.MonitorType != MonitorNone {
		t.Fatalf("incorrect source defaults: %+v", items[0].Defaults)
	}
	data, _ := json.Marshal(items)
	if strings.Contains(string(data), list.APIKey) {
		t.Fatal("credentials exposed in discover defaults")
	}
	if err := manager.TrackDiscoverItem(ctx, item.ID); err == nil {
		t.Fatal("tracked title not in library")
	}
	stored, err := store.GetItem(ctx, item.ID)
	if err != nil || stored.Status != ItemStatusPending {
		t.Fatalf("failed track changed source: %+v %v", stored, err)
	}
	svc.existing = &movies.Movie{ID: "library-title", Title: "Title"}
	for range 2 {
		if err := manager.TrackDiscoverItem(ctx, item.ID); err != nil {
			t.Fatal(err)
		}
	}
	stored, err = store.GetItem(ctx, item.ID)
	if err != nil || stored.Status != ItemStatusAdded {
		t.Fatalf("tracking did not persist: %+v %v", stored, err)
	}
	gotList, err := store.Get(ctx, list.ID)
	if err != nil || gotList.QualityProfileID != list.QualityProfileID || gotList.LibraryPath != list.LibraryPath || gotList.SearchOnAdd {
		t.Fatalf("source configuration changed: %+v %v", gotList, err)
	}
}

func TestTrackingHandlerDoesNotExposeProviderDiagnostics(t *testing.T) {
	ctx := context.Background()
	store := discoverStore(t)
	list := &ImportList{ID: "source", Name: "Watchlist", ListType: ListTypeTMDbList, Enabled: true, Mode: ListModeDiscover, MediaType: MediaTypeMovie}
	if err := store.Create(ctx, list); err != nil {
		t.Fatal(err)
	}
	item := &ImportListItem{ID: "item", ListID: list.ID, ExternalID: "tt123", IMDbID: "tt123", Title: "Title", Status: ItemStatusPending}
	if err := store.UpsertItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "private provider diagnostics", http.StatusUnauthorized)
	}))
	defer upstream.Close()
	manager := NewSyncManager(store, nil)
	manager.SetTMDBClient(tmdb.NewClient(tmdb.Config{APIKey: "private-test-key", BaseURL: upstream.URL}))
	recorder := httptest.NewRecorder()
	Router(store, manager, nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/discover/track", strings.NewReader(`{"item_id":"item"}`)))
	if recorder.Code != http.StatusConflict || strings.Contains(recorder.Body.String(), "private") || strings.Contains(recorder.Body.String(), upstream.URL) {
		t.Fatalf("unsafe tracking response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestDiscoverTrackingScopesMembershipByMedia(t *testing.T) {
	ctx := context.Background()
	store := discoverStore(t)
	list := &ImportList{ID: "source", Name: "TV", ListType: ListTypeTMDbList, Enabled: true, Mode: ListModeDiscover, MediaType: MediaTypeSeries, QualityProfileID: "quality", MonitorType: MonitorAll}
	if err := store.Create(ctx, list); err != nil {
		t.Fatal(err)
	}
	item := &ImportListItem{ID: "item", ListID: list.ID, ExternalID: "42", TMDbID: "42", Title: "Series", Status: ItemStatusPending}
	if err := store.UpsertItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	manager := NewSyncManager(store, nil)
	manager.SetMoviesService(&discoverMovies{existing: &movies.Movie{ID: "movie-with-same-provider-id"}})
	shows := &discoverSeries{}
	manager.SetSeriesService(shows)
	if err := manager.TrackDiscoverItem(ctx, item.ID); err == nil {
		t.Fatal("movie membership accepted for series")
	}
	id := "42"
	shows.existing = []*series.Series{{ID: "series", TMDBID: &id}}
	if err := manager.TrackDiscoverItem(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
}
