package qbittorrent

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

// readMultipartFields drains an /api/v2/torrents/add request body and
// returns a flat map of every form field it contained (file parts are
// keyed by part name with value "file:<filename>:<bytes>").
func readMultipartFields(t *testing.T, r *http.Request) map[string]string {
	t.Helper()
	ct := r.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		t.Fatalf("not multipart: %q", ct)
	}
	out := map[string]string{}
	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		body, _ := io.ReadAll(part)
		if part.FileName() != "" {
			out[part.FormName()] = fmt.Sprintf("file:%s:%s", part.FileName(), string(body))
		} else {
			out[part.FormName()] = string(body)
		}
	}
	return out
}

func TestAddMagnet(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	var captured map[string]string
	f.mux.HandleFunc("/api/v2/torrents/add", f.requireSID(func(w http.ResponseWriter, r *http.Request) {
		captured = readMultipartFields(t, r)
		fmt.Fprint(w, "Ok.")
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	hash := "0123456789abcdef0123456789abcdef01234567"
	res, err := c.Add(context.Background(), downloads.AddRequest{
		Magnet:   "magnet:?xt=urn:btih:" + hash + "&dn=example",
		Category: "movies",
		SavePath: "/downloads/movies",
		Tags:     []string{"loom", "auto"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.ItemID != hash {
		t.Fatalf("ItemID = %q, want %q", res.ItemID, hash)
	}
	if got := captured["urls"]; !strings.Contains(got, "magnet:?xt=urn:btih:"+hash) {
		t.Fatalf("urls field = %q", got)
	}
	if captured["category"] != "movies" {
		t.Fatalf("category = %q", captured["category"])
	}
	if captured["savepath"] != "/downloads/movies" {
		t.Fatalf("savepath = %q", captured["savepath"])
	}
	if captured["tags"] != "loom,auto" {
		t.Fatalf("tags = %q", captured["tags"])
	}
	if captured["paused"] != "false" {
		t.Fatalf("paused = %q", captured["paused"])
	}
}

// minimalTorrent constructs a tiny but valid bencoded torrent file
// for testing. The infohash is the SHA-1 of the bencoded info dict.
func minimalTorrent() []byte {
	// d
	//   8:announce 7:http://x
	//   4:info d 6:length i1e 4:name 1:a 12:piece length i16384e 6:pieces 0: e
	// e
	body := "d8:announce9:http://x:14:info" // placeholder; we'll build properly below
	_ = body
	return []byte("d8:announce9:http://x:" +
		"4:infod6:lengthi1e4:name1:a12:piece lengthi16384e6:pieces0:ee")
}

func TestAddRawTorrent(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	var captured map[string]string
	f.mux.HandleFunc("/api/v2/torrents/add", f.requireSID(func(w http.ResponseWriter, r *http.Request) {
		captured = readMultipartFields(t, r)
		fmt.Fprint(w, "Ok.")
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	bytes := minimalTorrent()
	res, err := c.Add(context.Background(), downloads.AddRequest{
		Title:    "Example",
		RawBytes: bytes,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.ItemID == "" {
		t.Fatalf("expected an infohash from raw torrent, got empty")
	}
	got := captured["torrents"]
	if !strings.HasPrefix(got, "file:Example.torrent:") {
		t.Fatalf("torrents part = %q", got)
	}
	if !strings.Contains(got, "d8:announce") {
		t.Fatalf("torrents body did not round-trip: %q", got)
	}
}

func TestAddURL(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	var captured map[string]string
	f.mux.HandleFunc("/api/v2/torrents/add", f.requireSID(func(w http.ResponseWriter, r *http.Request) {
		captured = readMultipartFields(t, r)
		fmt.Fprint(w, "Ok.")
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	res, err := c.Add(context.Background(), downloads.AddRequest{
		TorrentURL: "https://example.com/release.torrent",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.ClientID != "qb-test" {
		t.Fatalf("ClientID = %q", res.ClientID)
	}
	if got := captured["urls"]; got != "https://example.com/release.torrent" {
		t.Fatalf("urls = %q", got)
	}
	if res.ItemID != "" {
		t.Fatalf("ItemID for plain URL should be empty (qBittorrent does not echo it), got %q", res.ItemID)
	}
}

func TestAddAcceptsJSONSuccessPayload(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	f.mux.HandleFunc("/api/v2/torrents/add", f.requireSID(func(w http.ResponseWriter, r *http.Request) {
		_ = readMultipartFields(t, r)
		fmt.Fprint(w, `{"added_torrent_ids":["10c2701e43a9d8399c4582b1c74d1471c919b957"],"failure_count":0,"pending_count":0,"success_count":1}`)
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	_, err := c.Add(context.Background(), downloads.AddRequest{
		Magnet: "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=example",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
}

func TestInfohashFromMagnet(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"magnet:?xt=urn:btih:ABCDEF0123456789ABCDEF0123456789ABCDEF01": "abcdef0123456789abcdef0123456789abcdef01",
		"magnet:?dn=foo": "",
	}
	for in, want := range cases {
		if got := infohashFromMagnet(in); got != want {
			t.Errorf("infohashFromMagnet(%q) = %q, want %q", in, got, want)
		}
	}
}
