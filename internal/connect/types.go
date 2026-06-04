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
	ProviderTrakt    ProviderType = "trakt"
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
	// Trakt OAuth2 fields
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenExpiry  string `json:"token_expiry,omitempty"`
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

// Session is a single active playback reported by a media server. The
// connection-identifying fields are populated by callers; providers fill the
// media/playback fields.
type Session struct {
	ConnectionID     string       `json:"connection_id"`
	ConnectionName   string       `json:"connection_name"`
	Provider         ProviderType `json:"provider"`
	SessionKey       string       `json:"session_key"`
	MediaID          string       `json:"media_id"`
	User             string       `json:"user"`
	MediaType        string       `json:"media_type"` // movie | episode | other
	Title            string       `json:"title"`
	GrandparentTitle string       `json:"grandparent_title,omitempty"`
	FullTitle        string       `json:"full_title"`
	Device           string       `json:"device,omitempty"`
	State            string       `json:"state"` // playing | paused
	PositionMs       int64        `json:"position_ms"`
	DurationMs       int64        `json:"duration_ms"`
	Transcode        bool         `json:"transcode"`
	BitrateKbps      int64        `json:"bitrate_kbps"` // streaming bitrate in kbps, 0 if unknown
}
