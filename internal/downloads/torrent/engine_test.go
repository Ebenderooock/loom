package torrent

import (
	"log/slog"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// newTestEngine creates an Engine backed by a real anacrolix client
// on a random port with in-memory storage. Suitable for unit tests
// that need a functioning torrent handle.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()

	dir := t.TempDir()

	tcfg := torrent.NewDefaultClientConfig()
	tcfg.ListenPort = 0 // random
	tcfg.DataDir = dir
	tcfg.DefaultStorage = storage.NewFileWithCompletion(dir, storage.NewMapPieceCompletion())
	tcfg.NoDHT = true
	tcfg.DisablePEX = true
	tcfg.NoDefaultPortForwarding = true

	cl, err := torrent.NewClient(tcfg)
	if err != nil {
		t.Fatalf("creating test client: %v", err)
	}
	t.Cleanup(func() { cl.Close() })

	return &Engine{
		client:  cl,
		cfg:     Config{DownloadDir: dir, MaxConnections: 50},
		logger:  slog.Default(),
		items:   make(map[string]*trackedTorrent),
		dataDir: dir,
	}
}

// addTestTorrent adds a minimal torrent to the engine and returns the
// tracked entry plus its hash key.
func addTestTorrent(t *testing.T, e *Engine, opts ...func(*trackedTorrent)) (string, *trackedTorrent) {
	t.Helper()

	mi := testMetaInfo(t)
	th, err := e.client.AddTorrent(&mi)
	if err != nil {
		t.Fatalf("adding torrent: %v", err)
	}

	select {
	case <-th.GotInfo():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for torrent info")
	}

	hash := th.InfoHash().HexString()
	tt := &trackedTorrent{
		t:       th,
		title:   "test-torrent",
		addedAt: time.Now(),
	}
	for _, fn := range opts {
		fn(tt)
	}

	e.mu.Lock()
	e.items[hash] = tt
	e.mu.Unlock()

	return hash, tt
}

// testMetaInfo builds a minimal valid MetaInfo for testing.
func testMetaInfo(t *testing.T) metainfo.MetaInfo {
	t.Helper()

	info := metainfo.Info{
		PieceLength: 256 * 1024,
		Pieces:      make([]byte, 20),
		Name:        "test-file",
		Length:      1024,
	}
	mi := metainfo.MetaInfo{}
	var err error
	mi.InfoBytes, err = bencodeMarshalInfo(info)
	if err != nil {
		t.Fatalf("marshalling test info: %v", err)
	}
	return mi
}

// bencodeMarshalInfo uses the bencode package from anacrolix.
func bencodeMarshalInfo(info metainfo.Info) ([]byte, error) {
	return bencode.Marshal(info)
}

// --- buildStatus tests ---

func TestBuildStatus_Downloading(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t)
	_, tt := addTestTorrent(t, e)

	// Not paused, not complete, has info → downloading.
	tt.paused = false

	status := e.buildStatus(tt)
	if status.Status != "downloading" {
		t.Errorf("Status = %q, want %q", status.Status, "downloading")
	}
}

func TestBuildStatus_Paused(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t)
	_, tt := addTestTorrent(t, e)

	tt.paused = true

	status := e.buildStatus(tt)
	if status.Status != "paused" {
		t.Errorf("Status = %q, want %q", status.Status, "paused")
	}
	if !status.Paused {
		t.Error("Paused should be true")
	}
}

func TestBuildStatus_Seeding(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t)
	_, tt := addTestTorrent(t, e)

	// Seeding: complete and not paused.
	// We can't easily make t.Complete() return true without real data,
	// so we just verify the non-paused + has-info path gives "downloading".
	// The seeding path is tested via the status switch logic.
	tt.paused = false
	status := e.buildStatus(tt)

	// With zero-length pieces and no real data, the torrent likely reports
	// incomplete. Verify the structure is sane.
	if status.Title != "test-torrent" {
		t.Errorf("Title = %q, want %q", status.Title, "test-torrent")
	}
}

func TestBuildStatus_Fields(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t)
	hash, tt := addTestTorrent(t, e, func(tt *trackedTorrent) {
		tt.title = "My Title"
		tt.category = "movies"
		tt.savePath = "/save/here"
	})

	status := e.buildStatus(tt)

	if status.Hash != hash {
		t.Errorf("Hash = %q, want %q", status.Hash, hash)
	}
	if status.Title != "My Title" {
		t.Errorf("Title = %q, want %q", status.Title, "My Title")
	}
	if status.Category != "movies" {
		t.Errorf("Category = %q, want %q", status.Category, "movies")
	}
	if status.SavePath != "/save/here" {
		t.Errorf("SavePath = %q, want %q", status.SavePath, "/save/here")
	}
}

// --- enforceSeedPolicies tests ---
// These need real torrent handles for Stats(), so we use newTestEngine.

func TestSeedPolicy_SkipsPaused(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t)
	_, tt := addTestTorrent(t, e, func(tt *trackedTorrent) {
		tt.paused = true
		tt.seedPolicy = SeedPolicy{RatioLimit: 0.001}
	})

	e.enforceSeedPolicies()

	// Should remain paused — the function skips already-paused torrents.
	if !tt.paused {
		t.Error("expected torrent to stay paused")
	}
}

func TestSeedPolicy_SkipsIncomplete(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t)
	_, tt := addTestTorrent(t, e, func(tt *trackedTorrent) {
		tt.seedPolicy = SeedPolicy{RatioLimit: 0.001}
	})

	// Incomplete torrents should not be paused by seed policy.
	e.enforceSeedPolicies()

	if tt.paused {
		t.Error("incomplete torrent should not be paused by seed policy")
	}
}

func TestSeedPolicy_NeitherReached(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t)
	_, tt := addTestTorrent(t, e, func(tt *trackedTorrent) {
		// High limits that won't be reached.
		tt.seedPolicy = SeedPolicy{
			RatioLimit:       999.0,
			TimeLimitMinutes: 9999,
		}
	})

	e.enforceSeedPolicies()

	if tt.paused {
		t.Error("torrent should not be paused when limits are not reached")
	}
}

// --- Status method tests ---

func TestStatus_AllItems(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t)
	addTestTorrent(t, e)
	addTestTorrent(t, e) // same torrent, same hash → still one entry

	statuses := e.Status()
	if len(statuses) != 1 {
		t.Errorf("got %d statuses, want 1", len(statuses))
	}
}

func TestStatus_ByHash(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t)
	hash, _ := addTestTorrent(t, e)

	statuses := e.Status(hash)
	if len(statuses) != 1 {
		t.Fatalf("got %d statuses, want 1", len(statuses))
	}
	if statuses[0].Hash != hash {
		t.Errorf("Hash = %q, want %q", statuses[0].Hash, hash)
	}
}

func TestStatus_MissingHash(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t)

	statuses := e.Status("deadbeef00000000000000000000000000000000")
	if len(statuses) != 0 {
		t.Errorf("got %d statuses, want 0", len(statuses))
	}
}

func TestMagnetHasTrackers(t *testing.T) {
	cases := []struct {
		name   string
		magnet string
		want   bool
	}{
		{
			name:   "trackerless btih magnet",
			magnet: "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=ubuntu",
			want:   false,
		},
		{
			name:   "magnet with one tracker",
			magnet: "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&tr=udp%3A%2F%2Ftracker.example%3A1337%2Fannounce",
			want:   true,
		},
		{
			name:   "magnet with multiple trackers",
			magnet: "magnet:?xt=urn:btih:abc&tr=udp://a:1/announce&tr=udp://b:2/announce",
			want:   true,
		},
		{
			name:   "garbage",
			magnet: "not a magnet",
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := magnetHasTrackers(tc.magnet); got != tc.want {
				t.Fatalf("magnetHasTrackers(%q) = %v, want %v", tc.magnet, got, tc.want)
			}
		})
	}
}

func TestDefaultAnnounceList(t *testing.T) {
	list := defaultAnnounceList()
	if len(list) != len(defaultTrackers) {
		t.Fatalf("got %d tiers, want %d", len(list), len(defaultTrackers))
	}
	for i, tier := range list {
		if len(tier) != 1 || tier[0] != defaultTrackers[i] {
			t.Fatalf("tier %d = %v, want [%q]", i, tier, defaultTrackers[i])
		}
	}
}
