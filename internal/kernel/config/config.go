// Package config loads layered configuration:
//
//	defaults < $LOOM_CONFIG_DIR/loom.yaml (or --config) < environment (LOOM_*) < flags.
//
// Backed by spf13/viper. Hot-reload of safe-to-change keys (log.level,
// log.format) is gated by Config.HotReload and only fires when a YAML file
// was actually loaded.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// Config is the root of all runtime configuration.
type Config struct {
	ConfigDir string `mapstructure:"config_dir"`
	DataDir   string `mapstructure:"data_dir"`
	HotReload bool   `mapstructure:"hot_reload"`

	HTTP      HTTPConfig      `mapstructure:"http"`
	Log       LogConfig       `mapstructure:"log"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Auth      AuthConfig      `mapstructure:"auth"`
	Debug     DebugConfig     `mapstructure:"debug"`
	CORS      CORSConfig      `mapstructure:"cors"`
	OTel      OTelConfig      `mapstructure:"otel"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Indexers  IndexersConfig  `mapstructure:"indexers"`
	Downloads DownloadsConfig `mapstructure:"downloads"`
}

// IndexersConfig governs the indexer core (search fan-out + periodic
// health checks). All durations are seconds.
//
//   - SearchTimeoutSec is the per-indexer ceiling applied during a
//     fan-out search; an indexer that doesn't return inside the
//     ceiling contributes a per-source error and the rest of the
//     fan-out is unaffected.
//   - MaxParallel caps the number of concurrent in-flight indexer
//     calls during a fan-out.
//   - HealthCheckSchedule is a 5-field cron expression evaluated by
//     the scheduler (which is not running with WithSeconds=true). The
//     default fires every ten minutes.
//   - HealthCheckTimeoutSec bounds a single Test() call.
//   - Proxies governs Phase 2e per-indexer outbound proxies.
type IndexersConfig struct {
	SearchTimeoutSec      int              `mapstructure:"search_timeout"`
	MaxParallel           int              `mapstructure:"max_parallel"`
	HealthCheckSchedule   string           `mapstructure:"health_check_schedule"`
	HealthCheckTimeoutSec int              `mapstructure:"health_check_timeout"`
	Proxies               ProxiesConfig    `mapstructure:"proxies"`
	Cardigann             CardigannConfig  `mapstructure:"cardigann"`
}

// DownloadsConfig governs the download-client core (Phase 3a).
//
//   - OperationTimeoutSec is the per-client ceiling for fan-out
//     calls (status, free-space, test).
//   - MaxParallel caps concurrent in-flight client calls.
//   - HealthCheckSchedule is a 5-field cron expression (no seconds).
//     The default fires every five minutes — download clients tend
//     to be on the same network as Loom and tolerate frequent probes.
//   - HealthCheckTimeoutSec bounds a single Test() call.
type DownloadsConfig struct {
	OperationTimeoutSec   int    `mapstructure:"operation_timeout"`
	MaxParallel           int    `mapstructure:"max_parallel"`
	HealthCheckSchedule   string `mapstructure:"health_check_schedule"`
	HealthCheckTimeoutSec int    `mapstructure:"health_check_timeout"`
}

// CardigannConfig governs the Cardigann YAML definition loader
// (Phase 2b). DefinitionsDir defaults to <DataDir>/definitions/cardigann
// when left blank; relative paths are resolved against the working
// directory at boot.
type CardigannConfig struct {
	DefinitionsDir string `mapstructure:"definitions_dir"`
}

// ProxiesConfig configures the proxies subsystem (Phase 2e).
//
//   - FlareSolverrDefaultTimeoutSec applies when a FlareSolverr proxy
//     row leaves max_timeout_sec at zero. FlareSolverr exposes the
//     timeout in milliseconds; the package multiplies by 1000.
//   - TestProbeURL is the URL POST /api/v1/proxies/{id}/test fetches
//     to verify connectivity. Defaults to a small static endpoint.
type ProxiesConfig struct {
	FlareSolverrDefaultTimeoutSec int    `mapstructure:"flaresolverr_default_timeout"`
	TestProbeURL                  string `mapstructure:"test_probe_url"`
}

type HTTPConfig struct {
	Addr            string `mapstructure:"addr"`
	ReadTimeout     int    `mapstructure:"read_timeout"`
	WriteTimeout    int    `mapstructure:"write_timeout"`
	ShutdownTimeout int    `mapstructure:"shutdown_timeout"`
	URLBase         string `mapstructure:"url_base"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type TelemetryConfig struct {
	OTLPEndpoint string  `mapstructure:"otlp_endpoint"`
	Prometheus   bool    `mapstructure:"prometheus"`
	TraceRatio   float64 `mapstructure:"trace_ratio"`
	Profiling    bool    `mapstructure:"profiling"`
}

type DatabaseConfig struct {
	URL string `mapstructure:"url"`
}

// StorageConfig selects the persistence engine and engine-specific options.
// Engine must be "sqlite" or "postgres". Only the matching sub-block is
// consulted by storage.Open.
type StorageConfig struct {
	Engine   string         `mapstructure:"engine"`
	SQLite   SQLiteConfig   `mapstructure:"sqlite"`
	Postgres PostgresConfig `mapstructure:"postgres"`
}

type SQLiteConfig struct {
	Path string `mapstructure:"path"`
}

type PostgresConfig struct {
	DSN string `mapstructure:"dsn"`
}

type AuthConfig struct {
	Mode             string   `mapstructure:"mode"`
	TrustedProxyCIDR []string `mapstructure:"trusted_proxy_cidr"`

	SessionSecret string `mapstructure:"session_secret"`
	SessionTTL    int    `mapstructure:"session_ttl"`
	CookieSecure  bool   `mapstructure:"cookie_secure"`

	OIDC  OIDCConfig  `mapstructure:"oidc"`
	Proxy ProxyConfig `mapstructure:"proxy"`
}

type OIDCConfig struct {
	Enabled       bool     `mapstructure:"enabled"`
	IssuerURL     string   `mapstructure:"issuer_url"`
	ClientID      string   `mapstructure:"client_id"`
	ClientSecret  string   `mapstructure:"client_secret"`
	RedirectURL   string   `mapstructure:"redirect_url"`
	Scopes        []string `mapstructure:"scopes"`
	UsernameClaim string   `mapstructure:"username_claim"`
	EmailClaim    string   `mapstructure:"email_claim"`
	RoleClaim     string   `mapstructure:"role_claim"`
	AdminGroups   []string `mapstructure:"admin_groups"`
}

type ProxyConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	TrustedCIDRs []string `mapstructure:"trusted_cidrs"`
	UserHeader   string   `mapstructure:"user_header"`
	EmailHeader  string   `mapstructure:"email_header"`
	GroupsHeader string   `mapstructure:"groups_header"`
}

type DebugConfig struct {
	Pprof bool `mapstructure:"pprof"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// OTelConfig controls OpenTelemetry trace export. Prometheus metrics live
// under TelemetryConfig and are always exposed on /metrics regardless.
type OTelConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Endpoint string `mapstructure:"endpoint"`
}

// SchedulerConfig controls the persistent cron scheduler.
//
//   - Enabled gates the dispatch loop. False disables registration
//     side-effects too, useful for one-shot CLI subcommands that share
//     the same wiring.
//   - Timezone is the IANA name (or "Local") cron expressions are
//     interpreted in.
//   - ShutdownGrace bounds how long a SIGTERM waits for in-flight jobs
//     before abandoning them.
type SchedulerConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	Timezone      string `mapstructure:"timezone"`
	ShutdownGrace int    `mapstructure:"shutdown_grace"`
}

var (
	stateMu sync.Mutex
	state   *watchState
)

type watchState struct {
	v        *viper.Viper
	cfg      *Config
	handlers []func(*Config)
	started  bool
}

// Load returns a Config built from defaults < YAML file < env. The path
// argument, when non-empty, points at a YAML file and overrides the
// default lookup of $LOOM_CONFIG_DIR/loom.yaml.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("LOOM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	applyDefaults(v)
	bindLegacyEnv(v)

	if path != "" {
		v.SetConfigFile(path)
	} else {
		dir := envOr("LOOM_CONFIG_DIR", v.GetString("config_dir"))
		v.SetConfigName("loom")
		v.SetConfigType("yaml")
		v.AddConfigPath(dir)
	}
	if err := v.ReadInConfig(); err != nil {
		var nf viper.ConfigFileNotFoundError
		var pe *os.PathError
		// Missing file is fine; surface real parse errors only.
		if !errors.As(err, &nf) && !errors.As(err, &pe) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Resolve relative sqlite paths relative to config directory
	if cfg.Storage.SQLite.Path != "" && !filepath.IsAbs(cfg.Storage.SQLite.Path) && cfg.Storage.SQLite.Path != ":memory:" {
		cfg.Storage.SQLite.Path = filepath.Join(cfg.ConfigDir, cfg.Storage.SQLite.Path)
	} else if cfg.Storage.SQLite.Path == "" {
		// If no path configured, set it relative to config dir
		cfg.Storage.SQLite.Path = filepath.Join(cfg.ConfigDir, "data", "loom.db")
	}

	if err := os.MkdirAll(cfg.ConfigDir, 0o755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	stateMu.Lock()
	state = &watchState{v: v, cfg: cfg}
	stateMu.Unlock()
	return cfg, nil
}

// OnConfigChange registers a callback invoked with a freshly-unmarshaled
// Config every time the underlying YAML file changes. It is a no-op
// unless StartWatch has been called and a config file was actually loaded.
// Only safe-to-change keys (log.level, log.format) should be acted on by
// the callback.
func OnConfigChange(fn func(*Config)) {
	stateMu.Lock()
	defer stateMu.Unlock()
	if state == nil {
		return
	}
	state.handlers = append(state.handlers, fn)
}

// StartWatch enables hot-reload of the loaded config file. Idempotent;
// callable only after Load. Returns false if no file was loaded or watch
// is already running.
func StartWatch() bool {
	stateMu.Lock()
	defer stateMu.Unlock()
	if state == nil || state.started {
		return false
	}
	if state.v.ConfigFileUsed() == "" {
		return false
	}
	state.started = true
	state.v.OnConfigChange(func(_ fsnotify.Event) {
		stateMu.Lock()
		fresh := &Config{}
		if err := state.v.Unmarshal(fresh); err != nil {
			stateMu.Unlock()
			return
		}
		if err := fresh.Validate(); err != nil {
			stateMu.Unlock()
			return
		}
		state.cfg = fresh
		handlers := append([]func(*Config){}, state.handlers...)
		stateMu.Unlock()
		for _, h := range handlers {
			h(fresh)
		}
	})
	state.v.WatchConfig()
	return true
}

func applyDefaults(v *viper.Viper) {
	cwd, _ := os.Getwd()
	v.SetDefault("config_dir", filepath.Join(cwd, "config"))
	v.SetDefault("data_dir", filepath.Join(cwd, "config"))
	v.SetDefault("hot_reload", false)

	v.SetDefault("http.addr", ":8989")
	v.SetDefault("http.read_timeout", 30)
	v.SetDefault("http.write_timeout", 60)
	v.SetDefault("http.shutdown_timeout", 30)
	v.SetDefault("http.url_base", "")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	v.SetDefault("telemetry.prometheus", true)
	v.SetDefault("telemetry.trace_ratio", 0.0)
	v.SetDefault("telemetry.profiling", false)
	v.SetDefault("telemetry.otlp_endpoint", "")

	v.SetDefault("auth.mode", "forms")
	v.SetDefault("auth.trusted_proxy_cidr", []string{})
	v.SetDefault("auth.session_secret", "")
	v.SetDefault("auth.session_ttl", 30*24*3600)
	v.SetDefault("auth.cookie_secure", false)

	v.SetDefault("auth.oidc.enabled", false)
	v.SetDefault("auth.oidc.scopes", []string{"openid", "profile", "email"})
	v.SetDefault("auth.oidc.username_claim", "preferred_username")
	v.SetDefault("auth.oidc.email_claim", "email")
	v.SetDefault("auth.oidc.role_claim", "groups")
	v.SetDefault("auth.oidc.admin_groups", []string{})

	v.SetDefault("auth.proxy.enabled", false)
	v.SetDefault("auth.proxy.trusted_cidrs", []string{"127.0.0.1/32", "::1/128"})
	v.SetDefault("auth.proxy.user_header", "Remote-User")
	v.SetDefault("auth.proxy.email_header", "Remote-Email")
	v.SetDefault("auth.proxy.groups_header", "Remote-Groups")

	v.SetDefault("debug.pprof", false)

	// Default CORS origins for development (localhost:5173-5175 for frontend dev server)
	// Production deployments should configure this explicitly in loom.yaml
	v.SetDefault("cors.allowed_origins", []string{
		"http://localhost:5173",
		"http://localhost:5174",
		"http://localhost:5175",
		"http://localhost:3000",
	})

	v.SetDefault("otel.enabled", false)
	v.SetDefault("otel.endpoint", "")

	v.SetDefault("storage.engine", "sqlite")
	// Don't set a default sqlite path here; it's handled in Load() after file reading
	// v.SetDefault("storage.sqlite.path", "/data/loom.db")
	v.SetDefault("storage.postgres.dsn", "")

	v.SetDefault("scheduler.enabled", true)
	v.SetDefault("scheduler.timezone", "Local")
	v.SetDefault("scheduler.shutdown_grace", 30)

	v.SetDefault("indexers.search_timeout", 15)
	v.SetDefault("indexers.max_parallel", 8)
	v.SetDefault("indexers.health_check_schedule", "*/10 * * * *")
	v.SetDefault("indexers.health_check_timeout", 10)
	v.SetDefault("indexers.proxies.flaresolverr_default_timeout", 60)
	v.SetDefault("indexers.proxies.test_probe_url", "https://www.google.com/generate_204")
	v.SetDefault("indexers.cardigann.definitions_dir", "")

	v.SetDefault("downloads.operation_timeout", 15)
	v.SetDefault("downloads.max_parallel", 8)
	v.SetDefault("downloads.health_check_schedule", "*/5 * * * *")
	v.SetDefault("downloads.health_check_timeout", 10)
}

// bindLegacyEnv preserves the un-prefixed environment variable shape used
// before viper landed (LOOM_PROMETHEUS, LOOM_TRACE_RATIO, …) so existing
// deployments and tests keep working alongside the auto-bound shape
// (LOOM_TELEMETRY_PROMETHEUS, …).
func bindLegacyEnv(v *viper.Viper) {
	binds := map[string]string{
		"telemetry.prometheus":    "LOOM_PROMETHEUS",
		"telemetry.profiling":     "LOOM_PROFILING",
		"telemetry.trace_ratio":   "LOOM_TRACE_RATIO",
		"telemetry.otlp_endpoint": "LOOM_OTLP_ENDPOINT",
		"auth.mode":               "LOOM_AUTH_MODE",
		"database.url":            "LOOM_DATABASE_URL",
		"otel.enabled":            "LOOM_OTEL_ENABLED",
		"otel.endpoint":           "LOOM_OTEL_ENDPOINT",
	}
	for key, env := range binds {
		_ = v.BindEnv(key, env)
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
	switch strings.ToLower(c.Storage.Engine) {
	case "", "sqlite", "postgres":
	default:
		return fmt.Errorf("invalid storage.engine %q (want sqlite|postgres)", c.Storage.Engine)
	}
	if c.Indexers.SearchTimeoutSec < 0 {
		return fmt.Errorf("indexers.search_timeout must be non-negative")
	}
	if c.Indexers.MaxParallel < 0 {
		return fmt.Errorf("indexers.max_parallel must be non-negative")
	}
	if c.Indexers.HealthCheckTimeoutSec < 0 {
		return fmt.Errorf("indexers.health_check_timeout must be non-negative")
	}
	if c.Downloads.OperationTimeoutSec < 0 {
		return fmt.Errorf("downloads.operation_timeout must be non-negative")
	}
	if c.Downloads.MaxParallel < 0 {
		return fmt.Errorf("downloads.max_parallel must be non-negative")
	}
	if c.Downloads.HealthCheckTimeoutSec < 0 {
		return fmt.Errorf("downloads.health_check_timeout must be non-negative")
	}
	return nil
}

// parseBool retains its pre-viper behavior so callers (and tests) can keep
// poking at values that come in from non-viper sources (e.g. ad-hoc flags).
func parseBool(s string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
