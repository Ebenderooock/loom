// Package anime provides anime-specific release parsing, episode number
// mapping (absolute ↔ season-based), release-group quality scoring, and
// per-series preference persistence. It is designed to integrate with
// the import pipeline and custom format engine.
package anime

import (
	"database/sql/driver"
	"encoding/json"
)

// NumberingScheme controls how episode numbers are interpreted.
type NumberingScheme string

const (
	NumberingAbsolute NumberingScheme = "absolute"
	NumberingSeason   NumberingScheme = "season"
	NumberingAniDB    NumberingScheme = "anidb"
)

// SubType describes the subtitle style of a release.
type SubType string

const (
	SubTypeHardsub SubType = "hardsub"
	SubTypeSoftsub SubType = "softsub"
	SubTypeRaw     SubType = "raw"
)

// EpisodeMapping maps an absolute episode number to a season/episode pair.
type EpisodeMapping struct {
	AbsoluteNumber int    `json:"absoluteNumber"`
	SeasonNumber   int    `json:"seasonNumber"`
	EpisodeNumber  int    `json:"episodeNumber"`
	AniDBID        string `json:"anidbId,omitempty"`
	TVDBId         string `json:"tvdbId,omitempty"`
}

// AnimeRelease extends basic release info with anime-specific fields.
type AnimeRelease struct {
	// Base fields carried from the generic parser.
	Name       string `json:"name"`
	Title      string `json:"title"`
	Resolution int    `json:"resolution,omitempty"`
	Source     string `json:"source,omitempty"`
	Codec      string `json:"codec,omitempty"`
	Season     int    `json:"season"`
	Episode    int    `json:"episode"`
	Year       int    `json:"year,omitempty"`

	// Anime-specific fields.
	AbsoluteEpisode int     `json:"absoluteEpisode"`
	Version         int     `json:"version"`
	ReleaseGroup    string  `json:"releaseGroup"`
	IsDualAudio     bool    `json:"isDualAudio"`
	IsMultiAudio    bool    `json:"isMultiAudio"`
	SubType         SubType `json:"subType"`
	IsBatch         bool    `json:"isBatch"`
}

// ReleaseGroup describes a known anime release group and its quality ranking.
type ReleaseGroup struct {
	Name      string `json:"name"`
	Preferred bool   `json:"preferred"`
	Score     int    `json:"score"`
}

// AnimePreferences stores per-series anime configuration.
type AnimePreferences struct {
	SeriesID            string          `json:"seriesId"`
	NumberingScheme     NumberingScheme `json:"numberingScheme"`
	PreferredGroups     []string        `json:"preferredGroups"`
	DualAudioRequired   bool            `json:"dualAudioRequired"`
	ReleaseGroupScoring map[string]int  `json:"releaseGroupScoring,omitempty"`
}

// StringSliceJSON is a helper for JSON-encoded string slices in SQLite.
type StringSliceJSON []string

func (s StringSliceJSON) Value() (driver.Value, error) {
	if s == nil {
		return json.Marshal([]string{})
	}
	return json.Marshal(s)
}

func (s *StringSliceJSON) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return nil
	}
	return json.Unmarshal(b, s)
}

// IntMapJSON is a helper for JSON-encoded map[string]int in SQLite.
type IntMapJSON map[string]int

func (m IntMapJSON) Value() (driver.Value, error) {
	if m == nil {
		return json.Marshal(map[string]int{})
	}
	return json.Marshal(m)
}

func (m *IntMapJSON) Scan(value interface{}) error {
	if value == nil {
		*m = make(map[string]int)
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return nil
	}
	return json.Unmarshal(b, m)
}
