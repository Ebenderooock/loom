// Package config loads layered configuration:
//   defaults < $LOOM_CONFIG_DIR/loom.yaml < environment (LOOM_*) < flags.
//
// This Phase-0 implementation supports defaults + env. YAML loading and the
// hot-reload watcher land in Phase 1 alongside Viper.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is the root of all runtime configuration.
type Config struct {
	ConfigDir string
	DataDir   string

	HTTP      HTTPConfig
	Log       LogConfig
	Telemetry TelemetryConfig
	Database  DatabaseConfig
	Auth      AuthConfig
}

type HTTPConfig struct {
	Addr            string
	ReadTimeout     int // seconds
	WriteTimeout    int
	ShutdownTimeout int
	URLBase         string // e.g. "/loom" when reverse-proxied at a sub-path
}

type LogConfig struct {
	Level  string // debug | info | warn | error
	Format string // json | text
}

type TelemetryConfig struct {
	OTLPEndpoint string
	Prometheus   bool
	TraceRatio   float64
	Profiling    bool
}

type DatabaseConfig struct {
	URL string // empty => SQLite at <DataDir>/loom.db
}

type AuthConfig struct {
	Mode             string // forms | apikey | oidc | proxy | disabled (dev-only)
	TrustedProxyCIDR []string
}

// Load returns a Config built from defaults overlayed with environment.
// Path is reserved for the YAML file and is honored once Viper is wired in.
func Load(path string) (*Config, error) {
	cfg := defaults()

	if v := os.Getenv("LOOM_CONFIG_DIR"); v != "" {
		cfg.ConfigDir = v
	}
	if v := os.Getenv("LOOM_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("LOOM_HTTP_ADDR"); v != "" {
		cfg.HTTP.Addr = v
	}
	if v := os.Getenv("LOOM_HTTP_URL_BASE"); v != "" {
		cfg.HTTP.URLBase = v
	}
	if v := os.Getenv("LOOM_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("LOOM_LOG_FORMAT"); v != "" {
		cfg.Log.Format = v
	}
	if v := os.Getenv("LOOM_OTLP_ENDPOINT"); v != "" {
		cfg.Telemetry.OTLPEndpoint = v
	}
	if v := os.Getenv("LOOM_PROMETHEUS"); v != "" {
		cfg.Telemetry.Prometheus = parseBool(v, cfg.Telemetry.Prometheus)
	}
	if v := os.Getenv("LOOM_PROFILING"); v != "" {
		cfg.Telemetry.Profiling = parseBool(v, cfg.Telemetry.Profiling)
	}
	if v := os.Getenv("LOOM_TRACE_RATIO"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Telemetry.TraceRatio = f
		}
	}
	if v := os.Getenv("LOOM_DATABASE_URL"); v != "" {
		cfg.Database.URL = v
	}
	if v := os.Getenv("LOOM_AUTH_MODE"); v != "" {
		cfg.Auth.Mode = v
	}

	// path is honored in Phase 1 by Viper; for now ensure dirs exist.
	_ = path
	if err := os.MkdirAll(cfg.ConfigDir, 0o755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaults() *Config {
	cwd, _ := os.Getwd()
	return &Config{
		ConfigDir: filepath.Join(cwd, "config"),
		DataDir:   filepath.Join(cwd, "config"),
		HTTP: HTTPConfig{
			Addr:            ":8989",
			ReadTimeout:     30,
			WriteTimeout:    60,
			ShutdownTimeout: 30,
		},
		Log: LogConfig{Level: "info", Format: "json"},
		Telemetry: TelemetryConfig{
			Prometheus: true,
			TraceRatio: 0.0,
			Profiling:  false,
		},
		Auth: AuthConfig{Mode: "forms"},
	}
}

// Validate returns an error if cfg is internally inconsistent.
func (c *Config) Validate() error {
	switch strings.ToLower(c.Log.Level) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log.level %q", c.Log.Level)
	}
	switch strings.ToLower(c.Log.Format) {
	case "json", "text":
	default:
		return fmt.Errorf("invalid log.format %q", c.Log.Format)
	}
	if c.HTTP.Addr == "" {
		return fmt.Errorf("http.addr must not be empty")
	}
	if c.Telemetry.TraceRatio < 0 || c.Telemetry.TraceRatio > 1 {
		return fmt.Errorf("telemetry.trace_ratio must be in [0,1]")
	}
	switch c.Auth.Mode {
	case "forms", "apikey", "oidc", "proxy", "disabled":
	default:
		return fmt.Errorf("invalid auth.mode %q", c.Auth.Mode)
	}
	return nil
}

func parseBool(s string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}
