// Package apikeys provides standalone API key management and middleware
// for the Loom automation API surface.
package apikeys

import "time"

// APIKey represents a managed API key for automation clients.
type APIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"`
	Scopes    string     `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// APIKeyResponse is the masked version returned to API consumers.
type APIKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"` // masked
	Scopes    string     `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// MaskKey returns the first 8 chars + "..." of a key.
func MaskKey(k string) string {
	if len(k) <= 8 {
		return k
	}
	return k[:8] + "..."
}
