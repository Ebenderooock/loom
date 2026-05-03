package transmission

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/loomctl/loom/internal/downloads"
)

// TestStatusMappingCoversAllSevenStates feeds one synthetic torrent
// per Transmission status integer (0..6) and asserts the mapping
// onto downloads.ItemStatus matches the documented table.
func TestStatusMappingCoversAllSevenStates(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()

	cases := []struct {
		raw  int
		want downloads.ItemStatus
	}{
		{0, downloads.StatusItemPaused},
		{1, downloads.StatusItemQueued},
		{2, downloads.StatusItemDownloading},
		{3, downloads.StatusItemQueued},
		{4, downloads.StatusItemDownloading},
		{5, downloads.StatusItemQueued},
		{6, downloads.StatusItemSeeding},
	}

	rows := make([]map[string]any, 0, len(cases))
	for i, tc := range cases {
		rows = append(rows, map[string]any{
			"id":            i + 1,
			"hashString":    fixtureHash(i),
			"name":          "fixture",
			"status":        tc.raw,
			"totalSize":     int64(1000),
			"leftUntilDone": int64(250),
			"eta":           int64(60),
			"uploadRatio":   1.5,
			"percentDone":   0.75,
			"labels":        []string{"loom"},
			"rateDownload":  int64(1024),
			"rateUpload":    int64(2048),
		})
	}

	f.handle_("torrent-get", func(_ json.RawMessage) (any, string) {
		return map[string]any{"torrents": rows}, "success"
	})

	c := newTestClient(t, f, downloads.Definition{})
	items, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(items) != len(cases) {
		t.Fatalf("len = %d, want %d", len(items), len(cases))
	}
	for i, tc := range cases {
		if items[i].Status != tc.want {
			t.Errorf("status[%d]: raw=%d got=%q want=%q", i, tc.raw, items[i].Status, tc.want)
		}
	}
}

// TestStatusMapsRatioETAAndSize verifies the numeric projections, in
// particular the "downloaded = total - left" derivation and the
// negative-ETA-collapse-to-zero rule.
func TestStatusMapsRatioETAAndSize(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()
	f.handle_("torrent-get", func(_ json.RawMessage) (any, string) {
		return map[string]any{"torrents": []map[string]any{
			{
				"id": 1, "hashString": "AABB", "name": "t1", "status": 4,
				"totalSize": 1_000_000, "leftUntilDone": 250_000, "eta": -1,
				"uploadRatio": 0.42, "percentDone": 0.75, "labels": []string{"movies"},
				"rateDownload": 9_000, "rateUpload": 1_000,
			},
		}}, "success"
	})
	c := newTestClient(t, f, downloads.Definition{})
	items, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	got := items[0]
	if got.SizeBytes != 1_000_000 {
		t.Errorf("SizeBytes = %d", got.SizeBytes)
	}
	if got.DownloadedBytes != 750_000 {
		t.Errorf("DownloadedBytes = %d, want 750000", got.DownloadedBytes)
	}
	if got.ETA != 0 {
		t.Errorf("ETA = %d, want 0 (negative sentinel collapsed)", got.ETA)
	}
	if got.Ratio != 0.42 {
		t.Errorf("Ratio = %f", got.Ratio)
	}
	if got.Category != "movies" {
		t.Errorf("Category = %q, want movies (first label)", got.Category)
	}
	if got.ID != "aabb" {
		t.Errorf("ID = %q, want lowercased hashString", got.ID)
	}
}

// TestStatusFilteringByID asks for two ids and asserts the daemon
// receives them in the request envelope.
func TestStatusFilteringByID(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()

	var seen torrentGetArgs
	f.handle_("torrent-get", func(args json.RawMessage) (any, string) {
		_ = json.Unmarshal(args, &seen)
		return map[string]any{"torrents": []map[string]any{
			{"id": 1, "hashString": "abc", "name": "t1", "status": 4, "labels": []string{}},
		}}, "success"
	})
	c := newTestClient(t, f, downloads.Definition{})
	if _, err := c.Status(context.Background(), "abc", "def"); err != nil {
		t.Fatalf("Status: %v", err)
	}
	idsRaw, _ := json.Marshal(seen.IDs)
	if string(idsRaw) != `["abc","def"]` {
		t.Fatalf("ids = %s, want [abc,def]", idsRaw)
	}
}

// TestStatusEmptyForUnknownIDs surfaces the typed ErrUnknownTorrent
// when the caller asked about specific ids and the daemon returned
// nothing.
func TestStatusEmptyForUnknownIDs(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()
	f.handle_("torrent-get", func(_ json.RawMessage) (any, string) {
		return map[string]any{"torrents": []map[string]any{}}, "success"
	})
	c := newTestClient(t, f, downloads.Definition{})
	_, err := c.Status(context.Background(), "ghost-hash")
	if err == nil {
		t.Fatal("expected ErrUnknownTorrent, got nil")
	}
}

func fixtureHash(i int) string {
	const alphabet = "0123456789abcdef"
	out := make([]byte, 40)
	for j := range out {
		out[j] = alphabet[(i+j)%len(alphabet)]
	}
	return string(out)
}
