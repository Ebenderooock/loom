package appconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Library represents a media library folder configuration
type Library struct {
	Name string `json:"name"`
	Type string `json:"type"` // "tv", "movies", "music", etc.
	Path string `json:"path"`
}

// Admin holds admin user configuration
type Admin struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"` // bcrypt hash
}

// Persistence holds database configuration
type Persistence struct {
	Engine string `json:"engine"` // "sqlite"
	Path   string `json:"path"`
}

// RecycleBinConfig holds settings for the import recycle bin.
type RecycleBinConfig struct {
	Enabled       bool   `json:"enabled" yaml:"enabled"`
	Path          string `json:"path" yaml:"path"`
	RetentionDays int    `json:"retentionDays" yaml:"retentionDays"`
}

// Config is the root configuration structure
type Config struct {
	SetupComplete bool             `json:"setup_complete"`
	Admin         Admin            `json:"admin"`
	Persistence   Persistence      `json:"persistence"`
	Libraries     []Library        `json:"libraries"`
	RecycleBin    RecycleBinConfig `json:"recycle_bin"`
}

// Load reads and parses the config file from the given path
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate required fields
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate checks that the config has all required fields
func (c *Config) Validate() error {
	if c.Persistence.Engine == "" {
		return fmt.Errorf("persistence.engine is required")
	}
	if c.Persistence.Path == "" {
		return fmt.Errorf("persistence.path is required")
	}
	if c.SetupComplete {
		if c.Admin.Username == "" {
			return fmt.Errorf("admin.username is required when setup_complete is true")
		}
		if c.Admin.PasswordHash == "" {
			return fmt.Errorf("admin.password_hash is required when setup_complete is true")
		}
	}
	return nil
}

// Save writes the config to the given path
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// NewDefault returns a default configuration template for first-time setup
func NewDefault() *Config {
	return &Config{
		SetupComplete: false,
		Admin: Admin{
			Username:     "",
			PasswordHash: "",
		},
		Persistence: Persistence{
			Engine: "sqlite",
			Path:   "config/data/loom.db",
		},
		Libraries: []Library{},
	}
}
