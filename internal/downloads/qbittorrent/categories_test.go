package qbittorrent

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/downloads"
)

func TestCategories(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	f.mux.HandleFunc("/api/v2/torrents/categories", f.requireSID(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"movies": {"name": "movies", "savePath": "/downloads/movies"},
			"tv":     {"name": "tv",     "savePath": "/downloads/tv"},
			"linux":  {"name": "",       "savePath": "/downloads/linux"}
		}`)
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}
	cats, err := c.Categories(context.Background())
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("got %d categories, want 3", len(cats))
	}
	// Must be alphabetised.
	wantOrder := []string{"linux", "movies", "tv"}
	for i, c := range cats {
		if c.Name != wantOrder[i] {
			t.Errorf("cats[%d].Name = %q, want %q", i, c.Name, wantOrder[i])
		}
	}
	// Empty `name` field must fall back to the map key.
	if cats[0].Name != "linux" || cats[0].SavePath != "/downloads/linux" {
		t.Errorf("linux category mismapped: %+v", cats[0])
	}
}

func TestCategoriesEmpty(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	f.mux.HandleFunc("/api/v2/torrents/categories", f.requireSID(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}
	cats, err := c.Categories(context.Background())
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if len(cats) != 0 {
		t.Fatalf("want empty, got %v", cats)
	}
}

func TestCategoriesCacheInvalidatesAfterMutation(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()

	var categoryHits atomic.Int64
	f.mux.HandleFunc("/api/v2/torrents/categories", f.requireSID(func(w http.ResponseWriter, _ *http.Request) {
		categoryHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"movies":{"name":"movies","savePath":"/downloads/movies"}}`)
	}))
	f.mux.HandleFunc("/api/v2/torrents/pause", f.requireSID(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "Ok.")
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	now := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return now }
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}

	if _, err := c.Categories(context.Background()); err != nil {
		t.Fatalf("first Categories: %v", err)
	}
	if _, err := c.Categories(context.Background()); err != nil {
		t.Fatalf("second Categories: %v", err)
	}
	if got := categoryHits.Load(); got != 1 {
		t.Fatalf("category hits = %d, want 1", got)
	}

	if err := c.Pause(context.Background(), "abc"); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if _, err := c.Categories(context.Background()); err != nil {
		t.Fatalf("Categories after Pause: %v", err)
	}
	if got := categoryHits.Load(); got != 2 {
		t.Fatalf("category hits after invalidation = %d, want 2", got)
	}
}
