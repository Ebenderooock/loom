package torrent

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"ListenPort", cfg.ListenPort, 6881},
		{"SeedRatioLimit", cfg.SeedRatioLimit, 1.0},
		{"SeedTimeLimitMinutes", cfg.SeedTimeLimitMinutes, 0},
		{"MaxConnections", cfg.MaxConnections, 200},
		{"MaxUploadSlots", cfg.MaxUploadSlots, 50},
		{"EnableDHT", cfg.EnableDHT, true},
		{"EnablePEX", cfg.EnablePEX, true},
		{"EnableUPnP", cfg.EnableUPnP, false},
		{"DownloadSpeedLimit", cfg.DownloadSpeedLimit, int64(0)},
		{"UploadSpeedLimit", cfg.UploadSpeedLimit, int64(0)},
		{"MaxActiveTorrents", cfg.MaxActiveTorrents, 25},
		{"DownloadDir", cfg.DownloadDir, ""},
		{"IncompleteDir", cfg.IncompleteDir, ""},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestParseConfig_Empty(t *testing.T) {
	t.Parallel()

	def := downloads.Definition{
		SavePathDefault: "/data/downloads",
	}

	cfg, err := parseConfig(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use defaults plus fall back to SavePathDefault.
	if cfg.DownloadDir != "/data/downloads" {
		t.Errorf("DownloadDir = %q, want %q", cfg.DownloadDir, "/data/downloads")
	}
	if cfg.ListenPort != 6881 {
		t.Errorf("ListenPort = %d, want 6881", cfg.ListenPort)
	}
	if !cfg.EnableDHT {
		t.Error("EnableDHT should default to true")
	}
}

func TestParseConfig_Override(t *testing.T) {
	t.Parallel()

	blob := `{
		"listen_port": 9999,
		"download_dir": "/custom/dir",
		"enable_dht": false,
		"seed_ratio_limit": 2.5,
		"max_connections": 100
	}`

	def := downloads.Definition{
		Config: json.RawMessage(blob),
	}

	cfg, err := parseConfig(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenPort != 9999 {
		t.Errorf("ListenPort = %d, want 9999", cfg.ListenPort)
	}
	if cfg.DownloadDir != "/custom/dir" {
		t.Errorf("DownloadDir = %q, want %q", cfg.DownloadDir, "/custom/dir")
	}
	if cfg.EnableDHT {
		t.Error("EnableDHT should be overridden to false")
	}
	if cfg.SeedRatioLimit != 2.5 {
		t.Errorf("SeedRatioLimit = %f, want 2.5", cfg.SeedRatioLimit)
	}
	if cfg.MaxConnections != 100 {
		t.Errorf("MaxConnections = %d, want 100", cfg.MaxConnections)
	}
	// Un-overridden field should keep default.
	if !cfg.EnablePEX {
		t.Error("EnablePEX should remain true (default)")
	}
}

func TestParseConfig_DownloadDirRequired(t *testing.T) {
	t.Parallel()

	def := downloads.Definition{} // no config, no SavePathDefault

	_, err := parseConfig(def)
	if err == nil {
		t.Fatal("expected error when download_dir is missing")
	}
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("error = %v, want ErrNotConfigured", err)
	}
}

func TestParseConfig_FallbackSavePath(t *testing.T) {
	t.Parallel()

	// Config blob with no download_dir; should fall back to SavePathDefault.
	blob := `{"listen_port": 7000}`
	def := downloads.Definition{
		Config:          json.RawMessage(blob),
		SavePathDefault: "/fallback/path",
	}

	cfg, err := parseConfig(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DownloadDir != "/fallback/path" {
		t.Errorf("DownloadDir = %q, want %q", cfg.DownloadDir, "/fallback/path")
	}
	if cfg.ListenPort != 7000 {
		t.Errorf("ListenPort = %d, want 7000", cfg.ListenPort)
	}
}

func TestParseConfig_InvalidJSON(t *testing.T) {
	t.Parallel()

	def := downloads.Definition{
		Config: json.RawMessage(`{not valid json`),
	}

	_, err := parseConfig(def)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
