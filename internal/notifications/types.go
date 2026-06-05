package notifications

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// ConnectionType represents the kind of notification service.
type ConnectionType string

const (
	ConnectionTypeDiscord  ConnectionType = "discord"
	ConnectionTypeEmail    ConnectionType = "email"
	ConnectionTypeWebhook  ConnectionType = "webhook"
	ConnectionTypeSlack    ConnectionType = "slack"
	ConnectionTypeTelegram ConnectionType = "telegram"
	ConnectionTypeGotify   ConnectionType = "gotify"
	ConnectionTypePushover ConnectionType = "pushover"
	ConnectionTypeApprise  ConnectionType = "apprise"
	ConnectionTypeNtfy     ConnectionType = "ntfy"
)

// EventType represents a notification trigger event.
type EventType string

const (
	EventOnGrab              EventType = "on_grab"
	EventOnDownload          EventType = "on_download"
	EventOnUpgrade           EventType = "on_upgrade"
	EventOnRename            EventType = "on_rename"
	EventOnDelete            EventType = "on_delete"
	EventOnHealthIssue       EventType = "on_health_issue"
	EventOnApplicationUpdate EventType = "on_application_update"
	EventOnPlayback          EventType = "on_playback"
	EventOnTest              EventType = "on_test"
)

// Connection maps to the notification_connections table.
type Connection struct {
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	Type                ConnectionType     `json:"type"`
	Enabled             bool               `json:"enabled"`
	Settings            ConnectionSettings `json:"settings"`
	OnGrab              bool               `json:"on_grab"`
	OnDownload          bool               `json:"on_download"`
	OnUpgrade           bool               `json:"on_upgrade"`
	OnRename            bool               `json:"on_rename"`
	OnDelete            bool               `json:"on_delete"`
	OnHealthIssue       bool               `json:"on_health_issue"`
	OnApplicationUpdate bool               `json:"on_application_update"`
	OnPlayback          bool               `json:"on_playback"`
	Tags                StringSlice        `json:"tags"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
}

// SubscribesTo returns true if this connection is configured for the given event.
func (c *Connection) SubscribesTo(event EventType) bool {
	switch event {
	case EventOnGrab:
		return c.OnGrab
	case EventOnDownload:
		return c.OnDownload
	case EventOnUpgrade:
		return c.OnUpgrade
	case EventOnRename:
		return c.OnRename
	case EventOnDelete:
		return c.OnDelete
	case EventOnHealthIssue:
		return c.OnHealthIssue
	case EventOnApplicationUpdate:
		return c.OnApplicationUpdate
	case EventOnPlayback:
		return c.OnPlayback
	default:
		return false
	}
}

// HistoryEntry maps to the notification_history table.
type HistoryEntry struct {
	ID           int64     `json:"id"`
	ConnectionID *string   `json:"connection_id,omitempty"`
	EventType    string    `json:"event_type"`
	Title        string    `json:"title"`
	Message      string    `json:"message"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"`
	SentAt       time.Time `json:"sent_at"`
}

// Notification is the payload passed to senders.
type Notification struct {
	EventType EventType      `json:"event_type"`
	Title     string         `json:"title"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
}

// ConnectionSettings is a union type holding config for all connection types.
type ConnectionSettings struct {
	WebhookURL       string `json:"webhook_url,omitempty"`
	APIKey           string `json:"api_key,omitempty"`
	Channel          string `json:"channel,omitempty"`
	BotToken         string `json:"bot_token,omitempty"`
	ChatID           string `json:"chat_id,omitempty"`
	Host             string `json:"host,omitempty"`
	Port             int    `json:"port,omitempty"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	From             string `json:"from,omitempty"`
	To               string `json:"to,omitempty"`
	TLS              bool   `json:"tls,omitempty"`
	UserKey          string `json:"user_key,omitempty"`
	ServerURL        string `json:"server_url,omitempty"`
	Topic            string `json:"topic,omitempty"`
	TemplateOverride string `json:"template_override,omitempty"`
}

// Value implements driver.Valuer for database storage as JSON.
func (s ConnectionSettings) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan implements sql.Scanner for reading JSON from the database.
func (s *ConnectionSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		str, ok := value.(string)
		if !ok {
			return nil
		}
		b = []byte(str)
	}
	return json.Unmarshal(b, s)
}

// StringSlice is a JSON-marshaled string slice for database storage.
type StringSlice []string

// Value implements driver.Valuer.
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return json.Marshal([]string{})
	}
	return json.Marshal(s)
}

// Scan implements sql.Scanner.
func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
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

// CreateConnectionRequest is the API payload for creating a connection.
type CreateConnectionRequest struct {
	Name                string             `json:"name"`
	Type                ConnectionType     `json:"type"`
	Enabled             *bool              `json:"enabled,omitempty"`
	Settings            ConnectionSettings `json:"settings"`
	OnGrab              bool               `json:"on_grab"`
	OnDownload          bool               `json:"on_download"`
	OnUpgrade           bool               `json:"on_upgrade"`
	OnRename            bool               `json:"on_rename"`
	OnDelete            bool               `json:"on_delete"`
	OnHealthIssue       bool               `json:"on_health_issue"`
	OnApplicationUpdate bool               `json:"on_application_update"`
	OnPlayback          bool               `json:"on_playback"`
	Tags                []string           `json:"tags,omitempty"`
}

// UpdateConnectionRequest is the API payload for updating a connection.
type UpdateConnectionRequest struct {
	Name                *string             `json:"name,omitempty"`
	Type                *ConnectionType     `json:"type,omitempty"`
	Enabled             *bool               `json:"enabled,omitempty"`
	Settings            *ConnectionSettings `json:"settings,omitempty"`
	OnGrab              *bool               `json:"on_grab,omitempty"`
	OnDownload          *bool               `json:"on_download,omitempty"`
	OnUpgrade           *bool               `json:"on_upgrade,omitempty"`
	OnRename            *bool               `json:"on_rename,omitempty"`
	OnDelete            *bool               `json:"on_delete,omitempty"`
	OnHealthIssue       *bool               `json:"on_health_issue,omitempty"`
	OnApplicationUpdate *bool               `json:"on_application_update,omitempty"`
	OnPlayback          *bool               `json:"on_playback,omitempty"`
	Tags                []string            `json:"tags,omitempty"`
}
