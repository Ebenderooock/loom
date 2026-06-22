package torrent

import (
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

func TestParseConfigUsesSavePathDefault(t *testing.T) {
	def := downloads.Definition{
		ID:              "torrent-default",
		Kind:            Kind,
		SavePathDefault: "/media/downloads",
	}
	cfg, err := parseConfig(def)
	if err != nil {
		t.Fatalf("parseConfig error: %v", err)
	}
	if cfg.DownloadDir != "/media/downloads" {
		t.Fatalf("expected download dir from save_path_default, got %q", cfg.DownloadDir)
	}
	if cfg.rpcURL() != "http://127.0.0.1:7246" {
		t.Fatalf("unexpected default rpc url: %q", cfg.rpcURL())
	}
}

func TestBuildPublicMagnetIncludesTrackers(t *testing.T) {
	m := BuildPublicMagnet("abcdef0123456789abcdef0123456789abcdef01", "Some Title")
	if m == "" {
		t.Fatal("expected non-empty magnet")
	}
	if got := len(defaultTrackers); got == 0 {
		t.Fatal("expected default trackers to be configured")
	}
}
