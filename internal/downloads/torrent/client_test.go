package torrent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/ebenderooock/loom/internal/downloads"
)

// --- interface compliance ---

func TestClientInterfaceCompliance(t *testing.T) {
	t.Parallel()
	// Runtime check that *Client satisfies the interface.
	var c interface{} = &Client{}
	if _, ok := c.(downloads.DownloadClient); !ok {
		t.Fatal("*Client does not implement downloads.DownloadClient")
	}
}

// --- Add ---

func TestAdd_NoInput(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t)
	cl := &Client{
		id:     "test-id",
		name:   "test",
		engine: e,
		defConfig: Config{
			DownloadDir: e.dataDir,
		},
	}

	_, err := cl.Add(context.Background(), downloads.AddRequest{})
	if err == nil {
		t.Fatal("expected error for empty AddRequest")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("error = %v, want ErrInvalidInput", err)
	}
}

// --- fetchTorrentURL ---

func TestFetchTorrentURL_BadStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetchTorrentURL(context.Background(), srv.URL+"/test.torrent")
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404: %v", err)
	}
}

func TestFetchTorrentURL_TooLarge(t *testing.T) {
	t.Parallel()

	// Serve a response larger than 10 MiB.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		// Write just over the limit.
		data := make([]byte, maxTorrentFetchSize+1)
		w.Write(data)
	}))
	defer srv.Close()

	_, err := fetchTorrentURL(context.Background(), srv.URL+"/huge.torrent")
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("error should mention limit: %v", err)
	}
}

func TestFetchTorrentURL_InvalidTorrent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		fmt.Fprint(w, "this is not a torrent file")
	}))
	defer srv.Close()

	_, err := fetchTorrentURL(context.Background(), srv.URL+"/bad.torrent")
	if err == nil {
		t.Fatal("expected error for invalid torrent")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("error = %v, want ErrInvalidInput", err)
	}
}

func TestFetchTorrentURL_Success(t *testing.T) {
	t.Parallel()

	torrentData := buildMinimalTorrent(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		w.Write(torrentData)
	}))
	defer srv.Close()

	data, err := fetchTorrentURL(context.Background(), srv.URL+"/test.torrent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it parses back.
	mi, err := metainfo.Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("returned data is not valid torrent: %v", err)
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		t.Fatalf("cannot unmarshal info: %v", err)
	}
	if info.Name != "test-file" {
		t.Errorf("Name = %q, want %q", info.Name, "test-file")
	}
}

// --- mapEngineStatus ---

func TestMapEngineStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  downloads.ItemStatus
	}{
		{"queued", downloads.StatusItemQueued},
		{"downloading", downloads.StatusItemDownloading},
		{"seeding", downloads.StatusItemSeeding},
		{"paused", downloads.StatusItemPaused},
		{"completed", downloads.StatusItemCompleted},
		{"", downloads.StatusItemUnknown},
		{"some_unknown", downloads.StatusItemUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := mapEngineStatus(tc.input)
			if got != tc.want {
				t.Errorf("mapEngineStatus(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- Test (connectivity check) ---

func TestTest_MissingDir(t *testing.T) {
	t.Parallel()

	cl := &Client{
		id:   "test-id",
		name: "test",
		defConfig: Config{
			DownloadDir: "/nonexistent/path/that/should/not/exist",
		},
	}

	err := cl.Test(context.Background())
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

func TestTest_NotWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("skipping as root — file permission checks are bypassed")
	}
	t.Parallel()

	dir := t.TempDir()
	// Make the directory read-only.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	cl := &Client{
		id:   "test-id",
		name: "test",
		defConfig: Config{
			DownloadDir: dir,
		},
	}

	err := cl.Test(context.Background())
	if err == nil {
		t.Fatal("expected error for read-only directory")
	}
	if !strings.Contains(err.Error(), "not writable") {
		t.Errorf("error should mention 'not writable': %v", err)
	}
}

func TestTest_Success(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	cl := &Client{
		id:   "test-id",
		name: "test",
		defConfig: Config{
			DownloadDir: dir,
		},
	}

	if err := cl.Test(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Client accessor methods ---

func TestClientAccessors(t *testing.T) {
	t.Parallel()

	cl := &Client{
		id:   "my-id",
		name: "My Client",
	}

	if cl.ID() != "my-id" {
		t.Errorf("ID() = %q, want %q", cl.ID(), "my-id")
	}
	if cl.Name() != "My Client" {
		t.Errorf("Name() = %q, want %q", cl.Name(), "My Client")
	}
	if cl.Kind() != Kind {
		t.Errorf("Kind() = %q, want %q", cl.Kind(), Kind)
	}
	if cl.Protocol() != downloads.ProtocolTorrent {
		t.Errorf("Protocol() = %q, want %q", cl.Protocol(), downloads.ProtocolTorrent)
	}
}

// --- helpers ---

// buildMinimalTorrent creates a minimal valid .torrent file in memory.
func buildMinimalTorrent(t *testing.T) []byte {
	t.Helper()

	info := metainfo.Info{
		PieceLength: 256 * 1024,
		Pieces:      make([]byte, 20),
		Name:        "test-file",
		Length:      1024,
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatalf("marshalling info: %v", err)
	}

	mi := metainfo.MetaInfo{}
	mi.InfoBytes = infoBytes

	var buf bytes.Buffer
	if err := mi.Write(&buf); err != nil {
		t.Fatalf("writing metainfo: %v", err)
	}
	return buf.Bytes()
}
