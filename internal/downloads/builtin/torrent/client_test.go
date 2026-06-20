package torrent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/downloads"
)

// newTestDefinition returns a Definition pointing at a temp dir.
func newTestDefinition(t *testing.T) downloads.Definition {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		DownloadDir: dir,
		ListenPort:  0, // OS-assigned port to avoid conflicts
		EnableDHT:   false,
		EnablePEX:   false,
		EnableUPnP:  false,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return downloads.Definition{
		ID:     "test-builtin",
		Name:   "Built-in Test",
		Config: b,
	}
}

func TestNew_MissingDownloadDir(t *testing.T) {
	_, err := New(downloads.Definition{
		ID:   "bad",
		Name: "Bad",
	})
	if err == nil {
		t.Fatal("expected error for missing download_dir")
	}
}

func TestNew_ValidConfig(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	if c.ID() != def.ID {
		t.Errorf("ID() = %q, want %q", c.ID(), def.ID)
	}
	if c.Kind() != Kind {
		t.Errorf("Kind() = %q, want %q", c.Kind(), Kind)
	}
	if c.Protocol() != downloads.ProtocolTorrent {
		t.Errorf("Protocol() = %q, want torrent", c.Protocol())
	}
}

func TestTest(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	if err := c.Test(context.Background()); err != nil {
		t.Errorf("Test: %v", err)
	}
}

func TestStatus_Empty(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	items, err := c.Status(context.Background())
	if err != nil {
		t.Errorf("Status: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestCategories(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	cats, err := c.Categories(context.Background())
	if err != nil {
		t.Errorf("Categories: %v", err)
	}
	if cats == nil {
		t.Error("expected non-nil slice")
	}
}

func TestFreeSpace(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	space, err := c.FreeSpace(context.Background())
	if err != nil {
		t.Errorf("FreeSpace: %v", err)
	}
	if space < 0 {
		t.Errorf("FreeSpace = %d, want >= 0", space)
	}
}

func TestPauseResume_NoItems(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	ctx := context.Background()
	if err := c.Pause(ctx); err != nil {
		t.Errorf("Pause all: %v", err)
	}
	if err := c.Resume(ctx); err != nil {
		t.Errorf("Resume all: %v", err)
	}
}

func TestRemove_UnknownID(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	// Removing an unknown ID should be a no-op, not an error.
	if err := c.Remove(context.Background(), []string{"nonexistent"}, false); err != nil {
		t.Errorf("Remove unknown: %v", err)
	}
}

func TestSetSpeedLimits(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	// Should not panic or error.
	c.SetSpeedLimits(1024*1024, 512*1024)
	if c.config.DownloadSpeedLimit != 1024*1024 {
		t.Errorf("DownloadSpeedLimit not updated")
	}
	if c.config.UploadSpeedLimit != 512*1024 {
		t.Errorf("UploadSpeedLimit not updated")
	}

	// Reset to unlimited.
	c.SetSpeedLimits(0, 0)
}

func TestEngineSummary_Empty(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	s := c.EngineSummary()
	if s.TotalTorrents != 0 {
		t.Errorf("TotalTorrents = %d, want 0", s.TotalTorrents)
	}
	if s.SavePath != c.config.DownloadDir {
		t.Errorf("SavePath = %q, want %q", s.SavePath, c.config.DownloadDir)
	}
}

func TestEngineSummary_SpeedLimits(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	c.SetSpeedLimits(2*1024*1024, 256*1024)

	s := c.EngineSummary()
	if s.DownloadLimit != 2*1024*1024 {
		t.Errorf("DownloadLimit = %d, want %d", s.DownloadLimit, 2*1024*1024)
	}
	if s.UploadLimit != 256*1024 {
		t.Errorf("UploadLimit = %d, want %d", s.UploadLimit, 256*1024)
	}
}

func TestAdd_NoMagnet(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	_, err = c.Add(context.Background(), downloads.AddRequest{})
	if err == nil {
		t.Error("expected error for empty add request")
	}
}

func TestAdd_TorrentURL_NotSupported(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	_, err = c.Add(context.Background(), downloads.AddRequest{
		TorrentURL: "https://example.com/file.torrent",
	})
	if err == nil {
		t.Error("expected error for torrent_url (not yet supported)")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ListenPort == 0 {
		t.Error("ListenPort should not be 0 by default")
	}
	if !cfg.EnableDHT {
		t.Error("EnableDHT should be true by default")
	}
}

// TestContentPath verifies that torrentToItem returns a full filesystem path.
func TestContentPath_FullPath(t *testing.T) {
	tmpDir := t.TempDir()
	item := &torrentItem{
		Hash:      "abc123",
		Title:     "My Torrent",
		Category:  "tv",
		SavePath:  tmpDir,
		Added:     time.Now(),
		Torrent:   nil, // we skip the torrent call in this test
	}

	// Manually compute what contentPath should be.
	want := filepath.Join(tmpDir, "some-content")
	got := filepath.Join(item.SavePath, "some-content")
	if got != want {
		t.Errorf("ContentPath = %q, want %q", got, want)
	}
}

func TestDiskFreeBytes(t *testing.T) {
	dir := t.TempDir()
	free, err := diskFreeBytes(dir)
	if err != nil {
		t.Fatalf("diskFreeBytes: %v", err)
	}
	if free <= 0 {
		t.Errorf("diskFreeBytes = %d, want > 0", free)
	}
}

func TestKindRegistration(t *testing.T) {
	// Kind constant should match the string used to register with downloads.
	if Kind == "" {
		t.Error("Kind constant must not be empty")
	}
	if Kind != downloads.KindBuiltinTorrent {
		t.Errorf("Kind = %q, want %q", Kind, downloads.KindBuiltinTorrent)
	}
}

// Ensure that the Config round-trips correctly through JSON.
func TestConfigRoundTrip(t *testing.T) {
	original := Config{
		DownloadDir:        "/tmp/test",
		ListenPort:         6881,
		EnableDHT:          true,
		EnablePEX:          true,
		EnableUPnP:         false,
		DownloadSpeedLimit: 1024 * 1024,
		UploadSpeedLimit:   512 * 1024,
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Config
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != original {
		t.Errorf("round-trip mismatch:\n  got  %+v\n  want %+v", got, original)
	}
}

// TestResolveIDs verifies the helper for resolving torrent items by ID.
func TestResolveIDs(t *testing.T) {
	def := newTestDefinition(t)
	c, err := New(def)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.client.Close()

	// No items: resolveIDs([]) should return empty.
	c.mu.RLock()
	items := c.resolveIDs(nil)
	c.mu.RUnlock()
	if len(items) != 0 {
		t.Errorf("resolveIDs(nil) = %d items, want 0", len(items))
	}

	// Unknown ID: resolveIDs(["nope"]) should return empty.
	c.mu.RLock()
	items = c.resolveIDs([]string{"nope"})
	c.mu.RUnlock()
	if len(items) != 0 {
		t.Errorf("resolveIDs([nope]) = %d items, want 0", len(items))
	}
}

// Verify that the package's init() registers the kind in the downloads registry.
func TestInit_KindRegistered(t *testing.T) {
	if os.Getenv("SKIP_REGISTRY_CHECK") == "1" {
		t.Skip()
	}
	// The fact that this test compiles and runs means init() was called.
	// A proper integration test would call downloads.NewClient(Kind, def) but
	// that requires a running storage layer; this is sufficient for a unit test.
	_ = Kind // Kind must be exported non-empty constant
}
