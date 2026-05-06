package connect

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// ProviderType represents the kind of media server.
type ProviderType string

const (
	ProviderPlex     ProviderType = "plex"
	ProviderEmby     ProviderType = "emby"
	ProviderJellyfin ProviderType = "jellyfin"
)

// Connection maps to the connections table.
type Connection struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Provider       ProviderType     `json:"provider"`
	Enabled        bool             `json:"enabled"`
	Settings       ProviderSettings `json:"settings"`
	NotifyOnImport bool             `json:"notify_on_import"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

// ProviderSettings holds config for all provider types.
type ProviderSettings struct {
	Host   string `json:"host,omitempty"`
	APIKey string `json:"api_key,omitempty"`
}

// Value implements driver.Valuer for database storage as JSON.
func (s ProviderSettings) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan implements sql.Scanner for reading JSON from the database.
func (s *ProviderSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, s)
	case string:
		return json.Unmarshal([]byte(v), s)
	}
	return nil
}

// CreateRequest is the API payload for creating a connection.
type CreateRequest struct {
	Name           string           `json:"name"`
	Provider       ProviderType     `json:"provider"`
	Enabled        *bool            `json:"enabled,omitempty"`
	Settings       ProviderSettings `json:"settings"`
	NotifyOnImport *bool            `json:"notify_on_import,omitempty"`
}

// UpdateRequest is the API payload for updating a connection (pointer fields for partial updates).
type UpdateRequest struct {
	Name           *string           `json:"name,omitempty"`
	Provider       *ProviderType     `json:"provider,omitempty"`
	Enabled        *bool             `json:"enabled,omitempty"`
	Settings       *ProviderSettings `json:"settings,omitempty"`
	NotifyOnImport *bool             `json:"notify_on_import,omitempty"`
}
