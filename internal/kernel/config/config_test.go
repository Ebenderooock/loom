package config

import (
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
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
}

func TestEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOOM_CONFIG_DIR", filepath.Join(dir, "cfg"))
	t.Setenv("LOOM_DATA_DIR", filepath.Join(dir, "data"))
	t.Setenv("LOOM_HTTP_ADDR", "127.0.0.1:9000")
	t.Setenv("LOOM_LOG_LEVEL", "debug")
	t.Setenv("LOOM_PROMETHEUS", "false")
	t.Setenv("LOOM_TRACE_RATIO", "0.5")
	t.Setenv("LOOM_AUTH_MODE", "apikey")

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
			c := defaults()
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
