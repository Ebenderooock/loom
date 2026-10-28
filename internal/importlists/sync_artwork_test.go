package importlists

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

type artworkTransport func(*http.Request) (*http.Response, error)

func (f artworkTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestIMDbSyncArtwork(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(ctx, config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "artwork.db")},
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db.DB())
	list := &ImportList{
		Name: "IMDb artwork", ListType: ListTypeIMDbList, Enabled: true,
		URL: "https://www.imdb.com/list/ls123/", Mode: ListModeDiscover,
		MediaType: MediaTypeMovie, QualityProfileID: "default",
	}
	if err := store.Create(ctx, list); err != nil {
		t.Fatal(err)
	}

	transport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = transport })
	image := `"primaryImage":{"url":"https://m.media-amazon.com/images/poster.jpg"}`
	http.DefaultTransport = artworkTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://api.graphql.imdb.com/" {
			t.Fatalf("unexpected request: %s", req.URL)
		}
		body := `{"data":{"list":{"titleListItemSearch":{"edges":[{"title":{"id":"tt123","titleText":{"text":"Example"},"titleType":{"id":"movie"},` + image + `}}],"pageInfo":{"hasNextPage":false}}}}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	manager := NewSyncManager(store, logger)
	const poster = "https://m.media-amazon.com/images/poster.jpg"

	check := func(want string) *ImportListItem {
		t.Helper()
		if err := manager.SyncList(ctx, list); err != nil {
			t.Fatal(err)
		}
		items, err := store.ListItems(ctx, list.ID)
		if err != nil || len(items) != 1 {
			t.Fatalf("ListItems = (%v, %v), want one item", items, err)
		}
		if items[0].PosterPath != want {
			t.Fatalf("stored poster = %q, want %q", items[0].PosterPath, want)
		}
		rows, err := store.ListDiscoverItems(ctx, string(MediaTypeMovie))
		if err != nil || len(rows) != 1 || rows[0].Item.PosterPath != want {
			t.Fatalf("DiscoverItems = (%v, %v), want stored artwork %q", rows, err, want)
		}
		return items[0]
	}

	item := check(poster)
	item.PosterPath = ""
	if err := store.UpsertItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	item = check(poster)
	item.PosterPath = "https://image.tmdb.org/t/p/w342/cached.jpg"
	if err := store.UpsertItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	check(item.PosterPath)
	image = `"primaryImage":null`
	check(item.PosterPath)
}
