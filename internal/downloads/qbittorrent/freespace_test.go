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

func TestFreeSpaceCachesForOneMinute(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()

	var hits atomic.Int64
	f.mux.HandleFunc("/api/v2/sync/maindata", f.requireSID(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"server_state": {"free_space_on_disk": 5368709120}}`)
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	now := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return now }
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}

	if _, err := c.FreeSpace(context.Background()); err != nil {
		t.Fatalf("first FreeSpace: %v", err)
	}
	if _, err := c.FreeSpace(context.Background()); err != nil {
		t.Fatalf("second FreeSpace: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("free-space hits = %d, want 1", got)
	}

	now = now.Add(time.Minute + time.Second)
	if _, err := c.FreeSpace(context.Background()); err != nil {
		t.Fatalf("expired FreeSpace: %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("free-space hits after ttl = %d, want 2", got)
	}
}
