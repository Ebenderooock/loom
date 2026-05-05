// Package qualityprofiles manages quality profiles with per-profile
// custom format scoring, inspired by Radarr/Sonarr.
package qualityprofiles

import "time"

// QualityProfile defines quality preferences and custom format scoring.
type QualityProfile struct {
	ID                string       `json:"id"`
	Name              string       `json:"name"`
	Cutoff            string       `json:"cutoff"`
	MinFormatScore    int          `json:"min_format_score"`
	CutoffFormatScore int          `json:"cutoff_format_score"`
	UpgradeAllowed    bool         `json:"upgrade_allowed"`
	Items             string       `json:"items"` // JSON array of quality items
	FormatItems       []FormatItem `json:"format_items,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// FormatItem ties a custom format to a score within a quality profile.
type FormatItem struct {
	ProfileID string `json:"profile_id,omitempty"`
	FormatID  string `json:"format_id"`
	Score     int    `json:"score"`
}
