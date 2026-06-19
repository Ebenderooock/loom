package torrent

import (
	"context"
	"log/slog"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Kind is the kind string this package registers under.
const Kind = downloads.KindBuiltinTorrent

// Config is the settings shape for a built-in torrent client.
type Config struct {
	ListenPort              int    `json:"listen_port"`
	DownloadDir             string `json:"download_dir"`
	IncompleteDir           string `json:"incomplete_dir"`
	SeedRatioLimit          float64 `json:"seed_ratio_limit"`
	SeedTimeLimitMinutes    int    `json:"seed_time_limit_minutes"`
	MaxConnections          int    `json:"max_connections"`
	MaxUploadSlots          int    `json:"max_upload_slots"`
	EnableDHT               bool   `json:"enable_dht"`
	EnablePEX               bool   `json:"enable_pex"`
	EnableUPnP              bool   `json:"enable_upnp"`
	DownloadSpeedLimit      int64  `json:"download_speed_limit"`
	UploadSpeedLimit        int64  `json:"upload_speed_limit"`
	DebugPeerDiscovery      bool   `json:"debug_peer_discovery"`
}

// DefaultConfig returns sensible defaults for a new built-in torrent client.
func DefaultConfig() Config {
	return Config{
		ListenPort:          6881,
		SeedRatioLimit:      1.0,
		MaxConnections:      200,
		MaxUploadSlots:      50,
		EnableDHT:           true,
		EnablePEX:           true,
		EnableUPnP:          false,
		SeedTimeLimitMinutes: 0,
	}
}

func init() {
	logger := slog.Default()
	downloads.RegisterKind(Kind, func(ctx context.Context, def downloads.Definition) (downloads.DownloadClient, error) {
		logger.Debug("registering built-in torrent client", "id", def.ID)
		return New(def)
	})
}
