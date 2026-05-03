package nzbget

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/loomctl/loom/internal/downloads"
)

// queueGolden + historyGolden together cover the eight status
// classes the abstraction recognises plus a few NZBGet-specific
// post-processing states.
const queueGolden = `[
  {
    "NZBID": 101,
    "NZBName": "Movie.A.2024",
    "Kind": "NZB",
    "Status": "DOWNLOADING",
    "Category": "movies",
    "FileSizeMB": 1024,
    "RemainingSizeMB": 256,
    "DownloadedSizeMB": 768,
    "DownloadRate": 1048576,
    "DestDir": "/downloads/movies"
  },
  {
    "NZBID": 102,
    "NZBName": "Show.S01E01",
    "Kind": "NZB",
    "Status": "PAUSED",
    "Category": "tv",
    "FileSizeMB": 500,
    "RemainingSizeMB": 500,
    "DownloadedSizeMB": 0,
    "DownloadRate": 0
  },
  {
    "NZBID": 103,
    "NZBName": "Newest",
    "Kind": "NZB",
    "Status": "QUEUED",
    "Category": "tv",
    "FileSizeMB": 100,
    "RemainingSizeMB": 100
  },
  {
    "NZBID": 104,
    "NZBName": "Fetching.URL",
    "Kind": "URL",
    "Status": "FETCHING",
    "Category": "books",
    "FileSizeMB": 0,
    "RemainingSizeMB": 0
  },
  {
    "NZBID": 105,
    "NZBName": "Repair.Pack",
    "Kind": "NZB",
    "Status": "REPAIRING",
    "Category": "movies",
    "FileSizeMB": 700,
    "RemainingSizeMB": 0,
    "DownloadedSizeMB": 700,
    "PostInfoText": "Repairing PAR2 set"
  }
]`

const historyGolden = `[
  {
    "NZBID": 200,
    "Name": "Old.Show",
    "Kind": "NZB",
    "Status": "SUCCESS",
    "Category": "tv",
    "FileSizeMB": 700,
    "DestDir": "/downloads/tv",
    "ParStatus": "SUCCESS",
    "UnpackStatus": "SUCCESS"
  },
  {
    "NZBID": 201,
    "Name": "Bad.Pack",
    "Kind": "NZB",
    "Status": "FAILURE",
    "Category": "movies",
    "FileSizeMB": 0,
    "ParStatus": "FAILURE",
    "HealthStatus": "BAD"
  },
  {
    "NZBID": 202,
    "Name": "Health.Bad",
    "Kind": "NZB",
    "Status": "HEALTH",
    "Category": "tv",
    "FileSizeMB": 100,
    "HealthStatus": "BAD"
  }
]`

func TestStatus_MergesListgroupsAndHistory(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.onFunc("listgroups", func(_ *testing.T, _ []any) (any, *rpcError, int) {
		var rows []listGroup
		_ = json.Unmarshal([]byte(queueGolden), &rows)
		return rows, nil, 200
	})
	f.onFunc("history", func(_ *testing.T, params []any) (any, *rpcError, int) {
		// history(false) — caller must pass the boolean.
		if len(params) != 1 {
			t.Errorf("history params = %v", params)
		}
		var rows []historyGroup
		_ = json.Unmarshal([]byte(historyGolden), &rows)
		return rows, nil, 200
	})

	c := newTestClient(t, f)
	items, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(items) != 8 {
		t.Fatalf("expected 8 items, got %d", len(items))
	}

	byID := map[string]downloads.Item{}
	for _, it := range items {
		byID[it.ID] = it
	}

	// Downloading: progress, size, ETA, rate.
	dl := byID["101"]
	if dl.Status != downloads.StatusItemDownloading {
		t.Errorf("101 status = %s", dl.Status)
	}
	if dl.SizeBytes != int64(1024)*1024*1024 {
		t.Errorf("101 size = %d", dl.SizeBytes)
	}
	if dl.DownloadedBytes != int64(768)*1024*1024 {
		t.Errorf("101 downloaded = %d", dl.DownloadedBytes)
	}
	// 256 MB remaining at 1 MiB/s → 256 * 1024 * 1024 / 1048576 = 256 s.
	if dl.ETA != 256 {
		t.Errorf("101 ETA = %d", dl.ETA)
	}
	if dl.DownloadRate != 1048576 {
		t.Errorf("101 rate = %d", dl.DownloadRate)
	}
	if dl.Progress < 0.74 || dl.Progress > 0.76 {
		t.Errorf("101 progress = %f", dl.Progress)
	}

	if byID["102"].Status != downloads.StatusItemPaused {
		t.Errorf("102 status = %s", byID["102"].Status)
	}
	if byID["103"].Status != downloads.StatusItemQueued {
		t.Errorf("103 status = %s", byID["103"].Status)
	}
	if byID["104"].Status != downloads.StatusItemDownloading {
		t.Errorf("104 (FETCHING) status = %s", byID["104"].Status)
	}
	if byID["105"].Status != downloads.StatusItemDownloading {
		t.Errorf("105 (REPAIRING) status = %s", byID["105"].Status)
	}
	if byID["105"].Message != "Repairing PAR2 set" {
		t.Errorf("105 PostInfoText dropped: %q", byID["105"].Message)
	}

	if byID["200"].Status != downloads.StatusItemCompleted {
		t.Errorf("200 (SUCCESS) status = %s", byID["200"].Status)
	}
	if byID["201"].Status != downloads.StatusItemFailed {
		t.Errorf("201 (FAILURE) status = %s", byID["201"].Status)
	}
	if byID["201"].Message != "FAILURE" && byID["201"].Message != "BAD" {
		t.Errorf("201 should surface diagnostic message, got %q", byID["201"].Message)
	}
	if byID["202"].Status != downloads.StatusItemFailed {
		t.Errorf("202 (HEALTH) status = %s", byID["202"].Status)
	}
}

func TestStatus_FilterByID(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.onFunc("listgroups", func(_ *testing.T, _ []any) (any, *rpcError, int) {
		var rows []listGroup
		_ = json.Unmarshal([]byte(queueGolden), &rows)
		return rows, nil, 200
	})
	f.on("history", []historyGroup{})

	c := newTestClient(t, f)
	items, err := c.Status(context.Background(), "101")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(items) != 1 || items[0].ID != "101" {
		t.Fatalf("filter mismatch: %+v", items)
	}
}

func TestStatus_FilterMissReturnsNotFound(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("listgroups", []listGroup{})
	f.on("history", []historyGroup{})

	c := newTestClient(t, f)
	_, err := c.Status(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStatusMappings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		nzb  string
		want downloads.ItemStatus
	}{
		{"QUEUED", downloads.StatusItemQueued},
		{"PAUSED", downloads.StatusItemPaused},
		{"DOWNLOADING", downloads.StatusItemDownloading},
		{"FETCHING", downloads.StatusItemDownloading},
		{"PP_QUEUED", downloads.StatusItemDownloading},
		{"LOADING_PARS", downloads.StatusItemDownloading},
		{"VERIFYING_SOURCES", downloads.StatusItemDownloading},
		{"REPAIRING", downloads.StatusItemDownloading},
		{"UNPACKING", downloads.StatusItemDownloading},
		{"MOVING", downloads.StatusItemDownloading},
		{"EXECUTING_SCRIPT", downloads.StatusItemDownloading},
		{"COPYING", downloads.StatusItemDownloading},
		{"RENAMING", downloads.StatusItemDownloading},
		{"PP_FINISHED", downloads.StatusItemCompleted},
		{"DELETED", downloads.StatusItemFailed},
		{"FROBOZZ", downloads.StatusItemUnknown},
	}
	for _, c := range cases {
		if got := mapQueueStatus(c.nzb); got != c.want {
			t.Errorf("mapQueueStatus(%q) = %s want %s", c.nzb, got, c.want)
		}
	}

	hist := []struct {
		nzb  string
		want downloads.ItemStatus
	}{
		{"SUCCESS", downloads.StatusItemCompleted},
		{"FAILURE", downloads.StatusItemFailed},
		{"HEALTH", downloads.StatusItemFailed},
		{"WARNING", downloads.StatusItemCompleted},
		{"DELETED", downloads.StatusItemFailed},
		{"WHO_KNOWS", downloads.StatusItemUnknown},
	}
	for _, c := range hist {
		if got := mapHistoryStatus(c.nzb); got != c.want {
			t.Errorf("mapHistoryStatus(%q) = %s want %s", c.nzb, got, c.want)
		}
	}
}

func TestMbToBytesAndETA(t *testing.T) {
	t.Parallel()
	if got := mbToBytes(0); got != 0 {
		t.Errorf("mbToBytes(0) = %d", got)
	}
	if got := mbToBytes(1); got != 1024*1024 {
		t.Errorf("mbToBytes(1) = %d", got)
	}
	if got := mbToBytes(-5); got != 0 {
		t.Errorf("mbToBytes(neg) should be 0, got %d", got)
	}
	if got := etaSeconds(0, 100); got != 0 {
		t.Errorf("etaSeconds(0,100) = %d", got)
	}
	if got := etaSeconds(1024, 0); got != 0 {
		t.Errorf("etaSeconds(1024,0) = %d", got)
	}
	if got := etaSeconds(1024, 1024); got != 1 {
		t.Errorf("etaSeconds(1024,1024) = %d", got)
	}
}
