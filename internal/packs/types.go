// Package packs provides multi-season pack detection, splitting logic,
// and history tracking for release packs (complete seasons, multi-season
// bundles, and episode ranges).
package packs

import "time"

// SeasonPack represents a detected season or multi-season pack release.
type SeasonPack struct {
	ID           string `json:"id"`
	SeriesID     string `json:"seriesId"`
	Season       int    `json:"season"`       // -1 for complete series pack
	EpisodeStart int    `json:"episodeStart"` // 0 for full season
	EpisodeEnd   int    `json:"episodeEnd"`
	Quality      string `json:"quality"`
	Size         int64  `json:"size"`
	Indexer      string `json:"indexer"`
	DownloadURL  string `json:"downloadUrl"`
	Title        string `json:"title"`
	IsFullSeason bool   `json:"isFullSeason"`
}

// PackType categorises the kind of pack detected.
type PackType string

const (
	PackTypeSingleSeason   PackType = "single_season"
	PackTypeMultiSeason    PackType = "multi_season"
	PackTypeCompleteSeries PackType = "complete_series"
	PackTypeEpisodeRange   PackType = "episode_range"
)

// DetectedPack is the result of parsing a release title for pack info.
type DetectedPack struct {
	Type         PackType `json:"type"`
	SeasonStart  int      `json:"seasonStart"`
	SeasonEnd    int      `json:"seasonEnd"`
	EpisodeStart int      `json:"episodeStart"`
	EpisodeEnd   int      `json:"episodeEnd"`
	Title        string   `json:"title"`
	IsPack       bool     `json:"isPack"`
}

// PackDecision describes whether a pack should be grabbed vs individuals.
type PackDecision struct {
	ShouldGrabPack   bool    `json:"shouldGrabPack"`
	WantedInPack     int     `json:"wantedInPack"`
	TotalInPack      int     `json:"totalInPack"`
	WantedPercentage float64 `json:"wantedPercentage"`
	PackCostPerEp    float64 `json:"packCostPerEp"`
	IndividualCost   float64 `json:"individualCost"`
	Reason           string  `json:"reason"`
}

// PackHistory records that a pack was grabbed for a series.
type PackHistory struct {
	ID               string    `json:"id"`
	SeriesID         string    `json:"seriesId"`
	Season           int       `json:"season"`
	PackTitle        string    `json:"packTitle"`
	EpisodesIncluded []int     `json:"episodesIncluded"`
	Quality          string    `json:"quality"`
	GrabbedAt        time.Time `json:"grabbedAt"`
}
