package torrent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Config is the JSON shape persisted in download_clients.config for
// builtin/torrent rows. The implementation now talks to a Rain sidecar
// over JSON-RPC instead of embedding a BitTorrent engine in-process.
type Config struct {
	// Rain RPC endpoint configuration.
	RPCHost string `json:"rpc_host"`
	RPCPort int    `json:"rpc_port"`
	RPCURL  string `json:"rpc_url"`

	// Download directory (shared with Rain sidecar).
	DownloadDir string `json:"download_dir"`

	// DataDirIncludesTorrentID mirrors Rain's data-dir-includes-torrent-id
	// setting. When true (Rain's default), each torrent's files are stored
	// under {DownloadDir}/{torrentID}/ rather than directly in {DownloadDir}.
	// Loom must account for this when resolving on-disk content paths.
	DataDirIncludesTorrentID bool `json:"data_dir_includes_torrent_id"`

	// Optional metadata retained for compatibility with older
	// builtin/torrent settings and UI controls.
	SeedRatioLimit       float64 `json:"seed_ratio_limit"`
	SeedTimeLimitMinutes int     `json:"seed_time_limit_minutes"`
	DownloadSpeedLimit   int64   `json:"download_speed_limit"`
	UploadSpeedLimit     int64   `json:"upload_speed_limit"`
	PortBegin            int     `json:"port_begin"`
	EnableDHT            bool    `json:"enable_dht"`
	EnablePEX            bool    `json:"enable_pex"`
	EnableUPnP           bool    `json:"enable_upnp"`

	// RPC timeout for Rain requests.
	RequestTimeoutSecs int `json:"request_timeout_secs"`
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

func parseBool(s string) (bool, error) {
	return strconv.ParseBool(s)
}

// DefaultConfig returns defaults that match a local Rain sidecar.
func DefaultConfig() Config {
	return Config{
		RPCHost:                  "127.0.0.1",
		RPCPort:                  7246,
		SeedRatioLimit:           1.0,
		SeedTimeLimitMinutes:     0,
		DownloadSpeedLimit:       0,
		UploadSpeedLimit:         0,
		PortBegin:                6881,
		EnableDHT:                true,
		EnablePEX:                true,
		EnableUPnP:               false,
		DataDirIncludesTorrentID: true,
		RequestTimeoutSecs:       10,
	}
}

func (c Config) rpcURL() string {
	if strings.TrimSpace(c.RPCURL) != "" {
		return c.RPCURL
	}
	host := strings.TrimSpace(c.RPCHost)
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.RPCPort
	if port <= 0 {
		port = 7246
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

func (c Config) validate() error {
	if c.DownloadDir == "" {
		return fmt.Errorf("%w: download_dir (or save_path_default) is required", ErrNotConfigured)
	}
	u, err := url.Parse(c.rpcURL())
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%w: invalid rain rpc url %q", ErrNotConfigured, c.rpcURL())
	}
	if c.RequestTimeoutSecs <= 0 {
		c.RequestTimeoutSecs = 10
	}
	return nil
}

func applyEnv(cfg *Config) {
	if host := strings.TrimSpace(os.Getenv("LOOM_RAIN_RPC_HOST")); host != "" {
		cfg.RPCHost = host
	}
	if portStr := strings.TrimSpace(os.Getenv("LOOM_RAIN_RPC_PORT")); portStr != "" {
		if p, err := parseInt(portStr); err == nil && p > 0 {
			cfg.RPCPort = p
		}
	}
	if rpcURL := strings.TrimSpace(os.Getenv("LOOM_RAIN_RPC_URL")); rpcURL != "" {
		cfg.RPCURL = rpcURL
	}
	if t := strings.TrimSpace(os.Getenv("LOOM_RAIN_RPC_TIMEOUT_SECS")); t != "" {
		if v, err := parseInt(t); err == nil && v > 0 {
			cfg.RequestTimeoutSecs = v
		}
	}
	if dl := strings.TrimSpace(os.Getenv("LOOM_TORRENT_DOWNLOAD_DIR")); dl != "" {
		cfg.DownloadDir = dl
	}
	// Keep these env names for compatibility with existing Helm values.
	if dht := strings.TrimSpace(os.Getenv("LOOM_TORRENT_ENABLE_DHT")); dht != "" {
		if v, err := parseBool(dht); err == nil {
			cfg.EnableDHT = v
		}
	}
	if pex := strings.TrimSpace(os.Getenv("LOOM_TORRENT_ENABLE_PEX")); pex != "" {
		if v, err := parseBool(pex); err == nil {
			cfg.EnablePEX = v
		}
	}
	if inc := strings.TrimSpace(os.Getenv("LOOM_TORRENT_DATA_DIR_INCLUDES_ID")); inc != "" {
		if v, err := parseBool(inc); err == nil {
			cfg.DataDirIncludesTorrentID = v
		}
	}
}

func timeout(cfg Config) time.Duration {
	n := cfg.RequestTimeoutSecs
	if n <= 0 {
		n = 10
	}
	return time.Duration(n) * time.Second
}

// parseConfig merges Definition settings, config blob, and env overrides.
func parseConfig(def downloads.Definition) (Config, error) {
	cfg := DefaultConfig()

	if len(def.Config) > 0 {
		if err := json.Unmarshal(def.Config, &cfg); err != nil {
			return Config{}, fmt.Errorf("builtin/torrent: parsing config blob: %w", err)
		}
	}

	// Respect top-level host/port fields when present.
	if strings.TrimSpace(cfg.RPCHost) == "" && strings.TrimSpace(def.Host) != "" {
		cfg.RPCHost = strings.TrimSpace(def.Host)
	}
	if cfg.RPCPort <= 0 && def.Port > 0 {
		cfg.RPCPort = def.Port
	}
	if cfg.DownloadDir == "" && def.SavePathDefault != "" {
		cfg.DownloadDir = def.SavePathDefault
	}

	applyEnv(&cfg)

	if cfg.RequestTimeoutSecs <= 0 {
		cfg.RequestTimeoutSecs = 10
	}
	if cfg.RPCPort <= 0 {
		cfg.RPCPort = 7246
	}
	if cfg.RPCHost == "" {
		cfg.RPCHost = "127.0.0.1"
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
