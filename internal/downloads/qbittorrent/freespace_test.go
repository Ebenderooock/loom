package qbittorrent

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

func TestFreeSpace(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	f.mux.HandleFunc("/api/v2/sync/maindata", f.requireSID(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"server_state": {"free_space_on_disk": 5368709120}}`)
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}
	got, err := c.FreeSpace(context.Background())
	if err != nil {
		t.Fatalf("FreeSpace: %v", err)
	}
	if got != 5368709120 {
		t.Fatalf("FreeSpace = %d, want 5368709120", got)
	}
}

func TestFreeSpaceMissingFieldReturnsMinusOne(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	// Older qBittorrent versions ship server_state without
	// free_space_on_disk; we must report -1 rather than fail.
	f.mux.HandleFunc("/api/v2/sync/maindata", f.requireSID(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"server_state": {}}`)
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}
	got, err := c.FreeSpace(context.Background())
	if err != nil {
		t.Fatalf("FreeSpace: %v", err)
	}
	if got != -1 {
		t.Fatalf("FreeSpace = %d, want -1", got)
	}
}
