package torrent

import (
	"encoding/json"
	"fmt"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Config is the JSON shape persisted in download_clients.config for
// builtin/torrent rows. Values here override Definition-level fields
// when both are present.
type Config struct {
	ListenPort           int     `json:"listen_port"`
	DownloadDir          string  `json:"download_dir"`
	IncompleteDir        string  `json:"incomplete_dir"`
	SeedRatioLimit       float64 `json:"seed_ratio_limit"`
	SeedTimeLimitMinutes int     `json:"seed_time_limit_minutes"`
	MaxConnections       int     `json:"max_connections"`
	MaxUploadSlots       int     `json:"max_upload_slots"`
	EnableDHT            bool    `json:"enable_dht"`
	EnablePEX            bool    `json:"enable_pex"`
	EnableUPnP           bool    `json:"enable_upnp"`
	DownloadSpeedLimit   int64   `json:"download_speed_limit"`
	UploadSpeedLimit     int64   `json:"upload_speed_limit"`
	MaxActiveTorrents    int     `json:"max_active_torrents"`
}

// DefaultConfig returns sensible defaults for the built-in torrent
// engine: port 6881, DHT and PEX on, UPnP off, seed ratio 1.0, no
// time limit, 200 max connections, 50 max upload slots, unlimited
// speed, 25 max active torrents.
func DefaultConfig() Config {
	return Config{
		ListenPort:           6881,
		SeedRatioLimit:       1.0,
		SeedTimeLimitMinutes: 0,
		MaxConnections:       200,
		MaxUploadSlots:       50,
		EnableDHT:            true,
		EnablePEX:            true,
		EnableUPnP:           false,
		DownloadSpeedLimit:   0,
		UploadSpeedLimit:     0,
		MaxActiveTorrents:    25,
	}
}

// parseConfig merges the Definition-level fields with the JSON config
// blob. Config-blob values take precedence so that operators can drive
// everything through the config column if they prefer.
func parseConfig(def downloads.Definition) (Config, error) {
	cfg := DefaultConfig()

	if len(def.Config) > 0 {
		if err := json.Unmarshal(def.Config, &cfg); err != nil {
			return Config{}, fmt.Errorf("builtin/torrent: parsing config blob: %w", err)
		}
	}

	// Fall back to Definition-level save path when config does not
	// specify a download directory.
	if cfg.DownloadDir == "" && def.SavePathDefault != "" {
		cfg.DownloadDir = def.SavePathDefault
	}

	if cfg.DownloadDir == "" {
		return Config{}, fmt.Errorf("%w: download_dir (or save_path_default) is required", ErrNotConfigured)
	}

	return cfg, nil
}
