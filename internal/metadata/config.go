package metadata

import (
	"os"
	"strings"
	"time"
)

// Config holds runtime configuration for the metadata router and service.
type Config struct {
	// Providers is a comma-separated list of enabled metadata providers
	// (e.g., "tmdb,tvdb,musicbrainz"). Env: LOOM_METADATA_PROVIDERS
	Providers []string

	// Timeout is the total time allowed for a resolve call across all
	// providers (default: 10 seconds). Env: LOOM_METADATA_TIMEOUT
	Timeout time.Duration

	// CacheEnabled controls whether to use the in-process cache
	// (default: true).
	CacheEnabled bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Providers:    []string{"tmdb", "tvdb", "musicbrainz"},
		Timeout:      10 * time.Second,
		CacheEnabled: true,
	}
}

// ConfigFromEnv loads metadata configuration from environment variables.
// Falls back to defaults if env vars are not set.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	// LOOM_METADATA_PROVIDERS: comma-separated list
	if providers := os.Getenv("LOOM_METADATA_PROVIDERS"); providers != "" {
		parts := strings.Split(providers, ",")
		cleaned := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cleaned = append(cleaned, p)
			}
		}
		if len(cleaned) > 0 {
			cfg.Providers = cleaned
		}
	}

	// LOOM_METADATA_TIMEOUT: duration string (e.g., "10s", "15s")
	if timeout := os.Getenv("LOOM_METADATA_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil && d > 0 {
			cfg.Timeout = d
		}
	}

	// LOOM_METADATA_CACHE_ENABLED: "true" or "false" (defaults to true)
	if cacheDisabled := os.Getenv("LOOM_METADATA_CACHE_ENABLED"); cacheDisabled != "" {
		cfg.CacheEnabled = cacheDisabled != "false"
	}

	return cfg
}
