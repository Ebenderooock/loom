package qbittorrent

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

const torrentsInfoFixture = `[
  {
    "hash": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA1",
    "name": "Big Buck Bunny",
    "size": 1048576,
    "total_size": 2097152,
    "downloaded": 524288,
    "progress": 0.25,
    "state": "downloading",
    "category": "movies",
    "save_path": "/downloads/movies",
    "content_path": "/downloads/movies/Big Buck Bunny",
    "eta": 600,
    "dlspeed": 1024,
    "upspeed": 0,
    "ratio": 0.0,
    "amount_left": 1572864
  },
  {
    "hash": "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB2",
    "name": "Linux ISO",
    "size": 4096,
    "total_size": 4096,
    "downloaded": 4096,
    "progress": 1.0,
    "state": "uploading",
    "category": "linux",
    "save_path": "/downloads/linux",
    "eta": 0,
    "dlspeed": 0,
    "upspeed": 2048,
    "ratio": 1.5
  },
  {
    "hash": "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC3",
    "name": "Paused DL",
    "size": 1000,
    "total_size": 1000,
    "downloaded": 100,
    "progress": 0.1,
    "state": "pausedDL",
    "category": "",
    "save_path": "/downloads",
    "eta": 8640000,
    "dlspeed": 0,
    "upspeed": 0,
    "ratio": 0.0
  },
  {
    "hash": "DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD4",
    "name": "Paused UP (completed)",
    "size": 1000,
    "total_size": 1000,
    "downloaded": 1000,
    "progress": 1.0,
    "state": "pausedUP",
    "category": "tv",
    "save_path": "/downloads/tv",
    "eta": 0,
    "dlspeed": 0,
    "upspeed": 0,
    "ratio": 2.0
  },
  {
    "hash": "EEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE5",
    "name": "Errored",
    "size": 100,
    "total_size": 100,
    "downloaded": 0,
    "progress": 0.0,
    "state": "error",
    "category": "",
    "save_path": "/downloads",
    "eta": 0,
    "dlspeed": 0,
    "upspeed": 0,
    "ratio": 0.0
  },
  {
    "hash": "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF6",
    "name": "Stalled UP",
    "size": 100,
    "total_size": 100,
    "downloaded": 100,
    "progress": 1.0,
    "state": "stalledUP",
    "category": "",
    "save_path": "/downloads",
    "eta": 0,
    "dlspeed": 0,
    "upspeed": 0,
    "ratio": 3.0
  }
]`

func TestStatusGoldenFixture(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	f.mux.HandleFunc("/api/v2/torrents/info", f.requireSID(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, torrentsInfoFixture)
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}
	items, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(items) != 6 {
		t.Fatalf("got %d items, want 6", len(items))
	}

	wantStates := []downloads.ItemStatus{
		downloads.StatusItemDownloading,
		downloads.StatusItemSeeding,
		downloads.StatusItemPaused,
		downloads.StatusItemCompleted, // pausedUP collapses to completed
		downloads.StatusItemFailed,
		downloads.StatusItemSeeding,
	}
	for i, it := range items {
		if it.Status != wantStates[i] {
			t.Errorf("items[%d] state = %q, want %q (name=%q)",
				i, it.Status, wantStates[i], it.Title)
		}
	}

	// Spot-check field mapping on the first row.
	first := items[0]
	if first.SizeBytes != 2097152 {
		t.Errorf("size = %d", first.SizeBytes)
	}
	if first.DownloadedBytes != 524288 {
		t.Errorf("downloaded = %d", first.DownloadedBytes)
	}
	if first.ETA != 600 {
		t.Errorf("eta = %d", first.ETA)
	}
	if first.DownloadRate != 1024 {
		t.Errorf("dlspeed = %d", first.DownloadRate)
	}
	if first.Category != "movies" {
		t.Errorf("category = %q", first.Category)
	}
	if first.ContentPath != "/downloads/movies/Big Buck Bunny" {
		t.Errorf("content_path = %q", first.ContentPath)
	}
}

func TestStatusFiltersByHash(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	var sawHashes string
	f.mux.HandleFunc("/api/v2/torrents/info", f.requireSID(func(w http.ResponseWriter, r *http.Request) {
		sawHashes = r.URL.Query().Get("hashes")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"hash":"abc","name":"x","state":"downloading"}]`)
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := c.Status(context.Background(), "abc", "def"); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if sawHashes != "abc|def" {
		t.Fatalf("hashes filter = %q, want %q", sawHashes, "abc|def")
	}
}

func TestStatusUnknownHashErrors(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	f.mux.HandleFunc("/api/v2/torrents/info", f.requireSID(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}
	_, err := c.Status(context.Background(), "deadbeef")
	if err == nil {
		t.Fatal("expected ErrUnknownHash, got nil")
	}
}

func TestMapStateExhaustive(t *testing.T) {
	t.Parallel()
	cases := map[string]downloads.ItemStatus{
		"downloading":        downloads.StatusItemDownloading,
		"metaDL":             downloads.StatusItemDownloading,
		"forcedDL":           downloads.StatusItemDownloading,
		"checkingDL":         downloads.StatusItemDownloading,
		"allocating":         downloads.StatusItemDownloading,
		"stalledDL":          downloads.StatusItemDownloading,
		"queuedDL":           downloads.StatusItemQueued,
		"queuedUP":           downloads.StatusItemQueued,
		"uploading":          downloads.StatusItemSeeding,
		"forcedUP":           downloads.StatusItemSeeding,
		"stalledUP":          downloads.StatusItemSeeding,
		"checkingUP":         downloads.StatusItemSeeding,
		"pausedDL":           downloads.StatusItemPaused,
		"pausedUP":           downloads.StatusItemCompleted,
		"error":              downloads.StatusItemFailed,
		"missingFiles":       downloads.StatusItemFailed,
		"moving":             downloads.StatusItemDownloading,
		"checkingResumeData": downloads.StatusItemDownloading,
		"unrecognised":       downloads.StatusItemUnknown,
	}
	for in, want := range cases {
		if got := mapState(in); got != want {
			t.Errorf("mapState(%q) = %q, want %q", in, got, want)
		}
	}
}
