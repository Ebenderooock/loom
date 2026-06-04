// Package analytics provides Tautulli-style media-server analytics: live
// stream monitoring, persisted watch history, and watched reports built on top
// of the existing internal/connect media-server connections.
package analytics

import (
	"time"

	"github.com/ebenderooock/loom/internal/connect"
)

// HistoryRecord is a single persisted playback row in play_history. One
// continuous playback session maps to exactly one record.
type HistoryRecord struct {
	ID               string     `json:"id"`
	ConnectionID     string     `json:"connection_id"`
	Provider         string     `json:"provider"`
	SessionKey       string     `json:"session_key"`
	MediaID          string     `json:"media_id"`
	User             string     `json:"user"`
	MediaType        string     `json:"media_type"`
	Title            string     `json:"title"`
	GrandparentTitle string     `json:"grandparent_title"`
	FullTitle        string     `json:"full_title"`
	Device           string     `json:"device"`
	Transcode        bool       `json:"transcode"`
	StartedAt        time.Time  `json:"started_at"`
	LastSeenAt       time.Time  `json:"last_seen_at"`
	LastPositionMs   int64      `json:"last_position_ms"`
	DurationMs       int64      `json:"duration_ms"`
	WatchedMs        int64      `json:"watched_ms"`
	BitrateKbps      int64      `json:"bitrate_kbps"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
}

// LiveStream is an active session decorated with a progress percentage for the
// dashboard.
type LiveStream struct {
	connect.Session
	ConnectionName string  `json:"connection_name"`
	Progress       float64 `json:"progress"` // 0..100
}

// HistoryFilter narrows a history query.
type HistoryFilter struct {
	User   string
	Limit  int
	Offset int
}

// Totals summarises activity over a window.
type Totals struct {
	Plays          int   `json:"plays"`
	UniqueUsers    int   `json:"unique_users"`
	WatchedMs      int64 `json:"watched_ms"`
	TranscodePlays int   `json:"transcode_plays"`
	DirectPlays    int   `json:"direct_plays"`
	AvgBitrateKbps int64 `json:"avg_bitrate_kbps"` // average reported session bitrate (>0 only)
}

// UserStat is per-user aggregate activity.
type UserStat struct {
	User      string `json:"user"`
	Plays     int    `json:"plays"`
	WatchedMs int64  `json:"watched_ms"`
}

// MediaStat is per-title aggregate activity.
type MediaStat struct {
	MediaID   string `json:"media_id"`
	Title     string `json:"title"`
	MediaType string `json:"media_type"`
	Plays     int    `json:"plays"`
	WatchedMs int64  `json:"watched_ms"`
}

// DayCount is the play count for a single calendar day (UTC).
type DayCount struct {
	Day   string `json:"day"` // YYYY-MM-DD
	Plays int    `json:"plays"`
}

// Stats is the full analytics report for a window.
type Stats struct {
	WindowDays  int         `json:"window_days"`
	Totals      Totals      `json:"totals"`
	TopUsers    []UserStat  `json:"top_users"`
	TopMedia    []MediaStat `json:"top_media"`
	LeastMedia  []MediaStat `json:"least_media"`
	PlaysPerDay []DayCount  `json:"plays_per_day"`
}
