// Package bots implements the interactive Discord & Telegram request bots
// (Overseerr-equivalent chat front-ends) for Loom. Chat users link their
// account, search the metadata catalog, submit media requests, check status,
// and — for admins — approve or reject pending requests, all from chat.
//
// The package is split into a platform-agnostic "brain" (Service) that turns
// parsed commands into Reply values, thin transports (telegram, discord) that
// adapt each platform's API to the brain, and a Supervisor that starts and
// stops transports based on the stored configuration.
package bots

import "time"

// Platform identifies a chat platform.
type Platform string

const (
	PlatformTelegram Platform = "telegram"
	PlatformDiscord  Platform = "discord"
)

// Valid reports whether p is a recognised platform.
func (p Platform) Valid() bool {
	return p == PlatformTelegram || p == PlatformDiscord
}

// Config is the singleton bot configuration (bot_config row id=1). Tokens are
// stored as-is; callers exposing config over the API must mask them.
type Config struct {
	TelegramEnabled  bool   `json:"telegram_enabled"`
	TelegramBotToken string `json:"telegram_bot_token"`
	DiscordEnabled   bool   `json:"discord_enabled"`
	DiscordBotToken  string `json:"discord_bot_token"`

	// Default approval targets, applied when an admin approves a request from
	// chat (where there is no target picker as in the web UI).
	DefaultMovieQualityProfileID  string `json:"default_movie_quality_profile_id"`
	DefaultMovieLibraryID         string `json:"default_movie_library_id"`
	DefaultSeriesQualityProfileID string `json:"default_series_quality_profile_id"`
	DefaultSeriesLibraryID        string `json:"default_series_library_id"`

	UpdatedAt time.Time `json:"updated_at"`
}

// AccountLink binds a chat identity to a Loom user account.
type AccountLink struct {
	ID               string    `json:"id"`
	Platform         Platform  `json:"platform"`
	ExternalID       string    `json:"external_id"`
	ExternalUsername string    `json:"external_username"`
	UserID           int64     `json:"user_id"`
	CreatedAt        time.Time `json:"created_at"`
}

// LinkCode is a short-lived, single-use code that binds a chat identity to a
// Loom account when redeemed from the authenticated web UI.
type LinkCode struct {
	Code             string    `json:"code"`
	Platform         Platform  `json:"platform"`
	ExternalID       string    `json:"external_id"`
	ExternalUsername string    `json:"external_username"`
	ExpiresAt        time.Time `json:"expires_at"`
	CreatedAt        time.Time `json:"created_at"`
}

// LinkCodeTTL is how long a generated link code remains valid.
const LinkCodeTTL = 10 * time.Minute
