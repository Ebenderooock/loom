package qbittorrent

import (
	"context"
	"fmt"
	"net/http"
	"testing"

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
