package importlists

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/metadata/tmdb"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/storage"
)

type identityMovies struct {
	movies.Service
	adds int
}

func (s *identityMovies) GetMovieByTMDBID(_ context.Context, stringID string) (*movies.Movie, error) {
	if stringID != "550" {
		return nil, nil
	}
	id := "550"
	return &movies.Movie{ID: "fight-club-1999", TMDBID: &id}, nil
}

func (s *identityMovies) GetMovieByIMDBID(context.Context, string) (*movies.Movie, error) {
	return nil, nil
}

func (s *identityMovies) AddMovie(context.Context, *movies.Movie) error {
	s.adds++
	return nil
}

func TestDiscoverMatchesIMDbToTMDbLibraryMovie(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(ctx, config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "identity.db")},
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db.DB())
	list := &ImportList{Name: "IMDb", ListType: ListTypeIMDbList, Enabled: true, Mode: ListModeDiscover, MediaType: MediaTypeMovie}
	if err := store.Create(ctx, list); err != nil {
		t.Fatal(err)
	}
	item := &ImportListItem{
		ListID: list.ID, ExternalID: "tt0137523", IMDbID: "tt0137523",
		Title: "Fight Club", MediaType: "movie", Status: ItemStatusPending,
	}
	if err := store.UpsertItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/find/tt0137523" || r.URL.Query().Get("external_source") != "imdb_id" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		_, _ = io.WriteString(w, `{"movie_results":[{"id":550}]}`)
	}))
	defer server.Close()
	manager := NewSyncManager(store, logger)
	svc := &identityMovies{}
	manager.SetMoviesService(svc)
	manager.SetTMDBClient(tmdb.NewClient(tmdb.Config{APIKey: "test", BaseURL: server.URL}))
	for i := 0; i < 2; i++ {
		items, err := manager.DiscoverItems(ctx, "movie")
		if err != nil || len(items) != 1 || !items[0].InLibrary || items[0].TMDbID != "550" {
			t.Fatalf("DiscoverItems = (%v, %v), want existing library movie", items, err)
		}
	}
	if requests != 1 {
		t.Fatalf("identity was not cached: %d requests", requests)
	}
	stored, err := store.GetItem(ctx, item.ID)
	if err != nil || stored.TMDbID != "550" {
		t.Fatalf("resolved identity not persisted: (%v, %v)", stored, err)
	}
	// Exercise duplicate prevention independently of the Discover read.
	item.TMDbID = ""
	if err := manager.addMovieToLibrary(ctx, list, item); err != nil {
		t.Fatal(err)
	}
	if svc.adds != 0 {
		t.Fatalf("attempted %d duplicate additions", svc.adds)
	}
	if manager.itemInLibrary(ctx, "movie", &ImportListItem{
		Title: "Fight Club", TMDbID: "551", IMDbID: "tt9999999",
	}, nil) {
		t.Fatal("same title with different external IDs must not match")
	}
}
