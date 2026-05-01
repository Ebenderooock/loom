// Package config loads layered runtime configuration:
//
//	defaults < $LOOM_CONFIG_DIR/loom.yaml (or --config) < env (LOOM_*) < flags
//
// The implementation is backed by spf13/viper. Hot-reload of safe-to-change
// keys is gated behind Config.HotReload and only fires when a YAML file
// was actually loaded.
//
// The exhaustive key reference (name, env var, YAML path, default,
// hot-reloadable) lives in docs/configuration.md. The decision to use
// Viper for layered config is recorded in ADR-0001 (and ADR-0005 for the
// observability subset).
package config
