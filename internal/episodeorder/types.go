// Package episodeorder provides alternate episode ordering support
// (aired, DVD, absolute, scene) with mapping resolution and persistence.
package episodeorder

import "time"

// OrderingType identifies an episode numbering scheme.
type OrderingType string

const (
	OrderingAired    OrderingType = "aired"
	OrderingDVD      OrderingType = "dvd"
	OrderingAbsolute OrderingType = "absolute"
	OrderingScene    OrderingType = "scene"
)

// Valid returns true for known ordering types.
func (o OrderingType) Valid() bool {
	switch o {
	case OrderingAired, OrderingDVD, OrderingAbsolute, OrderingScene:
		return true
	}
	return false
}

// MappingSource describes where a mapping came from.
type MappingSource string

const (
	SourceManual MappingSource = "manual"
	SourceTVDB   MappingSource = "tvdb"
	SourceXEM    MappingSource = "xem"
	SourceAniDB  MappingSource = "anidb"
)

// EpisodeMapping maps an episode from one ordering scheme to another.
type EpisodeMapping struct {
	ID           string       `json:"id"`
	SeriesID     string       `json:"seriesId"`
	OrderingType OrderingType `json:"orderingType"`
	SeasonFrom   *int         `json:"seasonFrom,omitempty"`
	EpisodeFrom  *int         `json:"episodeFrom,omitempty"`
	AbsoluteFrom *int         `json:"absoluteFrom,omitempty"`
	SeasonTo     *int         `json:"seasonTo,omitempty"`
	EpisodeTo    *int         `json:"episodeTo,omitempty"`
	AbsoluteTo   *int         `json:"absoluteTo,omitempty"`
	Source       MappingSource `json:"source"`
	CreatedAt    time.Time    `json:"createdAt"`
}

// CreateMappingRequest is the JSON body for creating a mapping.
type CreateMappingRequest struct {
	OrderingType string `json:"orderingType"`
	SeasonFrom   *int   `json:"seasonFrom,omitempty"`
	EpisodeFrom  *int   `json:"episodeFrom,omitempty"`
	AbsoluteFrom *int   `json:"absoluteFrom,omitempty"`
	SeasonTo     *int   `json:"seasonTo,omitempty"`
	EpisodeTo    *int   `json:"episodeTo,omitempty"`
	AbsoluteTo   *int   `json:"absoluteTo,omitempty"`
	Source       string `json:"source,omitempty"`
}

// SetOrderingRequest is the JSON body for setting preferred ordering.
type SetOrderingRequest struct {
	OrderingType string `json:"orderingType"`
}
