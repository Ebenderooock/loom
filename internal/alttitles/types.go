package alttitles

import "time"

// AltTitle represents an alternate or original title for a movie or series.
type AltTitle struct {
	ID        string    `json:"id"`
	MediaID   string    `json:"media_id"`
	MediaType string    `json:"media_type"` // "movie" or "series"
	Title     string    `json:"title"`
	Language  string    `json:"language"`
	Source    string    `json:"source"` // "manual", "tmdb", "tvdb", "anidb"
	CreatedAt time.Time `json:"created_at"`
}

// CreateAltTitleRequest is the payload for adding an alt title.
type CreateAltTitleRequest struct {
	MediaID   string `json:"media_id"`
	MediaType string `json:"media_type"`
	Title     string `json:"title"`
	Language  string `json:"language,omitempty"`
	Source    string `json:"source,omitempty"`
}
