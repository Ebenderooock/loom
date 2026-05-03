package transmission

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/loomctl/loom/internal/downloads"
)

// TestAddMagnet verifies the magnet path: the magnet URI flows
// through `arguments.filename`, the category becomes a label, and
// the daemon's hashString is returned as the item id.
func TestAddMagnet(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()

	var seen torrentAddArgs
	f.handle_("torrent-add", func(args json.RawMessage) (any, string) {
		_ = json.Unmarshal(args, &seen)
		return map[string]any{
			"torrent-added": map[string]any{
				"id":         42,
				"hashString": "ABCDEF1234567890ABCDEF1234567890ABCDEF12",
				"name":       "Some.Show.S01E01",
			},
		}, "success"
	})

	c := newTestClient(t, f, downloads.Definition{})
	res, err := c.Add(context.Background(), downloads.AddRequest{
		Magnet:   "magnet:?xt=urn:btih:abcdef&dn=test",
		Category: "loom-tv",
		SavePath: "/downloads/loom",
		Tags:     []string{"paused", "extra-label"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.ItemID != "abcdef1234567890abcdef1234567890abcdef12" {
		t.Fatalf("ItemID = %q, want lower-cased hash", res.ItemID)
	}
	if seen.Filename != "magnet:?xt=urn:btih:abcdef&dn=test" {
		t.Fatalf("filename = %q", seen.Filename)
	}
	if seen.DownloadDir != "/downloads/loom" {
		t.Fatalf("download-dir = %q", seen.DownloadDir)
	}
	if !seen.Paused {
		t.Fatal("paused tag was not honoured")
	}
	if got, want := strings.Join(seen.Labels, ","), "loom-tv,extra-label"; got != want {
		t.Fatalf("labels = %q, want %q", got, want)
	}
}

// TestAddMetainfoBase64 verifies raw .torrent bytes are base64-encoded
// into `arguments.metainfo`.
func TestAddMetainfoBase64(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()

	raw := []byte("d4:infod4:name4:test6:lengthi42eee")
	want := base64.StdEncoding.EncodeToString(raw)

	var seen torrentAddArgs
	f.handle_("torrent-add", func(args json.RawMessage) (any, string) {
		_ = json.Unmarshal(args, &seen)
		return map[string]any{
			"torrent-added": map[string]any{
				"id":         1,
				"hashString": "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
				"name":       "from-bytes",
			},
		}, "success"
	})

	c := newTestClient(t, f, downloads.Definition{})
	res, err := c.Add(context.Background(), downloads.AddRequest{RawBytes: raw, Title: "from-bytes"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.ItemID == "" {
		t.Fatal("empty ItemID")
	}
	if seen.Metainfo != want {
		t.Fatalf("metainfo mismatch:\n got: %q\nwant: %q", seen.Metainfo, want)
	}
	if seen.Filename != "" {
		t.Fatalf("filename should be empty when metainfo is set, got %q", seen.Filename)
	}
}

// TestAddTorrentURL verifies fetchable URLs route through `filename`.
func TestAddTorrentURL(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()

	var seen torrentAddArgs
	f.handle_("torrent-add", func(args json.RawMessage) (any, string) {
		_ = json.Unmarshal(args, &seen)
		return map[string]any{
			"torrent-added": map[string]any{"id": 7, "hashString": "0000000000000000000000000000000000000001"},
		}, "success"
	})

	c := newTestClient(t, f, downloads.Definition{})
	if _, err := c.Add(context.Background(), downloads.AddRequest{
		TorrentURL: "https://tracker.example/dl?id=42",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if seen.Filename != "https://tracker.example/dl?id=42" {
		t.Fatalf("filename = %q", seen.Filename)
	}
}

// TestAddDuplicate handles the torrent-duplicate response shape — the
// daemon already has the torrent. We still return the hash.
func TestAddDuplicate(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()
	f.handle_("torrent-add", func(_ json.RawMessage) (any, string) {
		return map[string]any{
			"torrent-duplicate": map[string]any{
				"id": 9, "hashString": "FEEDFACEFEEDFACEFEEDFACEFEEDFACEFEEDFACE",
			},
		}, "success"
	})
	c := newTestClient(t, f, downloads.Definition{})
	res, err := c.Add(context.Background(), downloads.AddRequest{Magnet: "magnet:?xt=urn:btih:zz"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.ItemID != "feedfacefeedfacefeedfacefeedfacefeedface" {
		t.Fatalf("dup ItemID = %q", res.ItemID)
	}
}

// TestAddRequiresPayload surfaces the typed sentinel when none of the
// three input slots are populated.
func TestAddRequiresPayload(t *testing.T) {
	t.Parallel()
	f := newFakeDaemon(t)
	defer f.Close()
	c := newTestClient(t, f, downloads.Definition{})
	_, err := c.Add(context.Background(), downloads.AddRequest{})
	if err == nil {
		t.Fatal("expected error for empty AddRequest")
	}
	if !strings.Contains(err.Error(), "no magnet, torrent URL, or raw bytes") {
		t.Fatalf("error %q does not mention missing payload", err)
	}
}
