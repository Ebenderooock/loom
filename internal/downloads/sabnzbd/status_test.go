package sabnzbd

import (
	"context"
	"errors"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

const queueGolden = `{
  "queue": {
    "slots": [
      {
        "nzo_id": "SABnzbd_nzo_aaa",
        "filename": "Movie.A.2024",
        "cat": "movies",
        "status": "Downloading",
        "mb": "1024.0",
        "mbleft": "256.0",
        "percentage": "75",
        "timeleft": "0:05:00",
        "priority": "0",
        "script": "None"
      },
      {
        "nzo_id": "SABnzbd_nzo_bbb",
        "filename": "Show.S01E01",
        "cat": "tv",
        "status": "Paused",
        "mb": "500.0",
        "mbleft": "500.0",
        "percentage": "0",
        "timeleft": "0:00:00",
        "priority": "1",
        "script": "None"
      }
    ]
  }
}`

const historyGolden = `{
  "history": {
    "slots": [
      {
        "nzo_id": "SABnzbd_nzo_done",
        "name": "Old.Show",
        "category": "tv",
        "status": "Completed",
        "bytes": 700000000,
        "storage": "/downloads/Old.Show",
        "fail_message": ""
      },
      {
        "nzo_id": "SABnzbd_nzo_fail",
        "name": "Bad.Pack",
        "category": "movies",
        "status": "Failed",
        "bytes": 0,
        "storage": "",
        "fail_message": "PAR2 verification failed"
      }
    ]
  }
}`

func TestStatus_MergesQueueAndHistory(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("queue", queueGolden)
	f.on("history", historyGolden)

	c := newTestClient(t, f)
	items, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	byID := map[string]downloads.Item{}
	for _, it := range items {
		byID[it.ID] = it
	}

	dl := byID["SABnzbd_nzo_aaa"]
	if dl.Status != downloads.StatusItemDownloading {
		t.Errorf("downloading map: got %s", dl.Status)
	}
	if dl.Progress < 0.74 || dl.Progress > 0.76 {
		t.Errorf("progress = %f", dl.Progress)
	}
	if dl.SizeBytes != int64(1024)*1024*1024 {
		t.Errorf("size mismatch: got %d", dl.SizeBytes)
	}
	if dl.ETA != 300 {
		t.Errorf("eta = %d", dl.ETA)
	}
	if dl.Category != "movies" {
		t.Errorf("category = %q", dl.Category)
	}

	if byID["SABnzbd_nzo_bbb"].Status != downloads.StatusItemPaused {
		t.Errorf("paused map: %+v", byID["SABnzbd_nzo_bbb"])
	}
	if byID["SABnzbd_nzo_done"].Status != downloads.StatusItemCompleted {
		t.Errorf("completed map: %+v", byID["SABnzbd_nzo_done"])
	}
	failed := byID["SABnzbd_nzo_fail"]
	if failed.Status != downloads.StatusItemFailed {
		t.Errorf("failed map: %+v", failed)
	}
	if failed.Message != "PAR2 verification failed" {
		t.Errorf("fail message dropped: %q", failed.Message)
	}
}

func TestStatus_FilterByID(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("queue", queueGolden)
	f.on("history", historyGolden)

	c := newTestClient(t, f)
	items, err := c.Status(context.Background(), "SABnzbd_nzo_done")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(items) != 1 || items[0].ID != "SABnzbd_nzo_done" {
		t.Fatalf("filter mismatch: %+v", items)
	}
}

func TestStatus_FilterMissReturnsNotFound(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("queue", `{"queue":{"slots":[]}}`)
	f.on("history", `{"history":{"slots":[]}}`)

	c := newTestClient(t, f)
	_, err := c.Status(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStatusMappings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		sab  string
		want downloads.ItemStatus
	}{
		{"Downloading", downloads.StatusItemDownloading},
		{"Extracting", downloads.StatusItemDownloading},
		{"Paused", downloads.StatusItemPaused},
		{"Queued", downloads.StatusItemQueued},
		{"Completed", downloads.StatusItemCompleted},
		{"Failed", downloads.StatusItemFailed},
		{"Frobozz", downloads.StatusItemUnknown},
	}
	for _, c := range cases {
		if got := mapQueueStatus(c.sab); got != c.want {
			t.Errorf("mapQueueStatus(%q) = %s want %s", c.sab, got, c.want)
		}
	}
}

func TestParseTimeLeft(t *testing.T) {
	t.Parallel()
	cases := map[string]int64{
		"":        0,
		"0:00:00": 0,
		"0:05:00": 300,
		"1:00:00": 3600,
		"15:30":   930,
		"42":      42,
		"junk":    0,
	}
	for in, want := range cases {
		if got := parseTimeLeft(in); got != want {
			t.Errorf("parseTimeLeft(%q) = %d want %d", in, got, want)
		}
	}
}
