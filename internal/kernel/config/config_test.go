package config

import (
	"os"
	"path/filepath"
	"testing"
)

// clearEnv unsets every LOOM_* variable set elsewhere in the binary so
// test cases don't leak state. t.Setenv handles per-test reset, but we
// also need to neutralize any pre-existing values.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, e := range os.Environ() {
		k := e
		if i := indexByte(e, '='); i >= 0 {
			k = e[:i]
		}
		if len(k) > 5 && k[:5] == "LOOM_" {
			t.Setenv(k, "")
		}
	}
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("LOOM_CONFIG_DIR", t.TempDir())
	t.Setenv("LOOM_DATA_DIR", t.TempDir())

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != ":8989" {
		t.Errorf("default http.addr = %q, want :8989", cfg.HTTP.Addr)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("default log.level = %q, want info", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("default log.format = %q, want json", cfg.Log.Format)
	}
	if !cfg.Telemetry.Prometheus {
		t.Errorf("prometheus should default to true")
	}
	if cfg.Auth.Mode != "forms" {
		t.Errorf("default auth.mode = %q, want forms", cfg.Auth.Mode)
	}
	if cfg.OTel.Enabled {
		t.Errorf("otel should default to disabled")
	}
	if cfg.Debug.Pprof {
		t.Errorf("debug.pprof should default to false")
	}
	if cfg.HTTP.ReadTimeout != 30 {
		t.Errorf("http.read_timeout = %d, want 30", cfg.HTTP.ReadTimeout)
	}
}

func TestEnvOverridesLegacy(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Setenv("LOOM_CONFIG_DIR", filepath.Join(dir, "cfg"))
	t.Setenv("LOOM_DATA_DIR", filepath.Join(dir, "data"))
	t.Setenv("LOOM_HTTP_ADDR", "127.0.0.1:9000")
	t.Setenv("LOOM_LOG_LEVEL", "debug")
	t.Setenv("LOOM_PROMETHEUS", "false")
	t.Setenv("LOOM_TRACE_RATIO", "0.5")
	t.Setenv("LOOM_AUTH_MODE", "apikey")
	t.Setenv("LOOM_OTEL_ENABLED", "true")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != "127.0.0.1:9000" {
		t.Errorf("addr = %q", cfg.HTTP.Addr)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("level = %q", cfg.Log.Level)
	}
	if cfg.Telemetry.Prometheus {
		t.Errorf("prometheus should be false")
	}
	if cfg.Telemetry.TraceRatio != 0.5 {
		t.Errorf("trace ratio = %v", cfg.Telemetry.TraceRatio)
	}
	if cfg.Auth.Mode != "apikey" {
		t.Errorf("auth mode = %q", cfg.Auth.Mode)
	}
	if !cfg.OTel.Enabled {
		t.Errorf("otel.enabled should be true")
	}
}

func TestEnvOverridesNested(t *testing.T) {
	clearEnv(t)
	t.Setenv("LOOM_CONFIG_DIR", t.TempDir())
	t.Setenv("LOOM_DATA_DIR", t.TempDir())
	t.Setenv("LOOM_HTTP_URL_BASE", "/loom")
	t.Setenv("LOOM_DEBUG_PPROF", "true")
	t.Setenv("LOOM_HOT_RELOAD", "true")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.URLBase != "/loom" {
		t.Errorf("url_base = %q", cfg.HTTP.URLBase)
	}
	if !cfg.Debug.Pprof {
		t.Errorf("debug.pprof should be true")
	}
	if !cfg.HotReload {
		t.Errorf("hot_reload should be true")
	}
}

func TestLoadYAMLFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	yaml := []byte(`
http:
  addr: "127.0.0.1:1234"
log:
  level: warn
  format: text
cors:
  allowed_origins: ["https://example.com", "https://app.example.com"]
otel:
  enabled: true
  endpoint: "http://collector:4318"
`)
	path := filepath.Join(dir, "loom.yaml")
	if err := os.WriteFile(path, yaml, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOOM_CONFIG_DIR", t.TempDir())
	t.Setenv("LOOM_DATA_DIR", t.TempDir())

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != "127.0.0.1:1234" {
		t.Errorf("yaml http.addr = %q", cfg.HTTP.Addr)
	}
	if cfg.Log.Level != "warn" || cfg.Log.Format != "text" {
		t.Errorf("yaml log = %+v", cfg.Log)
	}
	if len(cfg.CORS.AllowedOrigins) != 2 {
		t.Errorf("yaml cors origins = %v", cfg.CORS.AllowedOrigins)
	}
	if !cfg.OTel.Enabled || cfg.OTel.Endpoint != "http://collector:4318" {
		t.Errorf("yaml otel = %+v", cfg.OTel)
	}
}

func TestEnvOverridesYAML(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	yaml := []byte("http:\n  addr: \"127.0.0.1:1111\"\n")
	path := filepath.Join(dir, "loom.yaml")
	if err := os.WriteFile(path, yaml, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOOM_CONFIG_DIR", t.TempDir())
	t.Setenv("LOOM_DATA_DIR", t.TempDir())
	t.Setenv("LOOM_HTTP_ADDR", "127.0.0.1:2222")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.Addr != "127.0.0.1:2222" {
		t.Errorf("env should override yaml: %q", cfg.HTTP.Addr)
	}
}

func TestValidateRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"bad log level", func(c *Config) { c.Log.Level = "trace" }},
		{"bad log format", func(c *Config) { c.Log.Format = "xml" }},
		{"empty addr", func(c *Config) { c.HTTP.Addr = "" }},
		{"trace ratio > 1", func(c *Config) { c.Telemetry.TraceRatio = 1.5 }},
		{"bad auth mode", func(c *Config) { c.Auth.Mode = "magic" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{
				HTTP: HTTPConfig{Addr: ":1"},
				Log:  LogConfig{Level: "info", Format: "json"},
				Auth: AuthConfig{Mode: "forms"},
			}
			tc.mut(c)
			if err := c.Validate(); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestParseBool(t *testing.T) {
	cases := map[string]bool{
		"true": true, "1": true, "yes": true, "on": true,
		"false": false, "0": false, "no": false, "off": false,
	}
	for in, want := range cases {
		if got := parseBool(in, !want); got != want {
			t.Errorf("parseBool(%q) = %v, want %v", in, got, want)
		}
	}
	if got := parseBool("garbage", true); got != true {
		t.Errorf("fallback should win on unknown input")
	}
}

func TestStartWatchNoFile(t *testing.T) {
	clearEnv(t)
	t.Setenv("LOOM_CONFIG_DIR", t.TempDir())
	t.Setenv("LOOM_DATA_DIR", t.TempDir())
	if _, err := Load(""); err != nil {
		t.Fatal(err)
	}
	// No YAML loaded → StartWatch is a no-op.
	if StartWatch() {
		t.Errorf("StartWatch should report false when no file is loaded")
	}
}
