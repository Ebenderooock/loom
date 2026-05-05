package series

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// SeriesStatus represents the current airing status of a series.
type SeriesStatus string

const (
	StatusContinuing SeriesStatus = "continuing"
	StatusEnded      SeriesStatus = "ended"
	StatusUpcoming   SeriesStatus = "upcoming"
	StatusCancelled  SeriesStatus = "cancelled"
)

// SeriesType represents the type of series.
type SeriesType string

const (
	TypeStandard SeriesType = "standard"
	TypeDaily    SeriesType = "daily"
	TypeAnime    SeriesType = "anime"
)

// MonitoringStatus represents the monitoring strategy for a series.
type MonitoringStatus string

const (
	MonitoringAll          MonitoringStatus = "all"
	MonitoringFuture       MonitoringStatus = "future"
	MonitoringMissing      MonitoringStatus = "missing"
	MonitoringExisting     MonitoringStatus = "existing"
	MonitoringPilot        MonitoringStatus = "pilot"
	MonitoringFirstSeason  MonitoringStatus = "firstSeason"
	MonitoringLastSeason   MonitoringStatus = "lastSeason"
	MonitoringNone         MonitoringStatus = "none"
	MonitoringMonitored    MonitoringStatus = "monitored"
	MonitoringUnmonitored  MonitoringStatus = "unmonitored"
)

// Series represents a TV series in the library.
type Series struct {
	ID               string           `json:"id"`
	Title            string           `json:"title"`
	Year             int              `json:"year,omitempty"`
	IMDBID           *string          `json:"imdb_id,omitempty"`
	TMDBID           *string          `json:"tmdb_id,omitempty"`
	TVDBID           *string          `json:"tvdb_id,omitempty"`
	Overview         string           `json:"overview,omitempty"`
	Genres           StringSlice      `json:"genres,omitempty"`
	Runtime          int              `json:"runtime,omitempty"`
	Rating           float64          `json:"rating,omitempty"`
	BackdropPath     string           `json:"backdrop_path,omitempty"`
	PosterPath       string           `json:"poster_path,omitempty"`
	Network          string           `json:"network,omitempty"`
	Status           SeriesStatus     `json:"status"`
	SeriesType       SeriesType       `json:"series_type"`
	MetadataProvider string           `json:"metadata_provider,omitempty"`
	QualityProfileID string           `json:"quality_profile_id,omitempty"`
	RootFolderID     string           `json:"root_folder_id,omitempty"`
	MonitoringStatus MonitoringStatus `json:"monitoring_status"`
	SeasonFolder     bool             `json:"season_folder"`
	ReleaseDate      string           `json:"release_date,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`

	// Populated on read, not stored directly
	Seasons  []*Season  `json:"seasons,omitempty"`
	Episodes []*Episode `json:"episodes,omitempty"`
}

// Season represents a season of a TV series.
type Season struct {
	ID           string    `json:"id"`
	SeriesID     string    `json:"series_id"`
	SeasonNumber int       `json:"season_number"`
	Title        string    `json:"title,omitempty"`
	Overview     string    `json:"overview,omitempty"`
	PosterPath   string    `json:"poster_path,omitempty"`
	Monitored    bool      `json:"monitored"`
	EpisodeCount int       `json:"episode_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Episode represents a single episode of a TV series.
type Episode struct {
	ID            string    `json:"id"`
	SeriesID      string    `json:"series_id"`
	SeasonID      string    `json:"season_id"`
	EpisodeNumber int       `json:"episode_number"`
	Title         string    `json:"title,omitempty"`
	Overview      string    `json:"overview,omitempty"`
	AirDate       string    `json:"air_date,omitempty"`
	Runtime       int       `json:"runtime,omitempty"`
	StillPath     string    `json:"still_path,omitempty"`
	Monitored     bool      `json:"monitored"`
	HasFile       bool      `json:"has_file"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// EpisodeFile represents a media file for an episode.
type EpisodeFile struct {
	ID         string    `json:"id"`
	EpisodeID  string    `json:"episode_id"`
	SeriesID   string    `json:"series_id"`
	FilePath   string    `json:"file_path"`
	FileSize   int64     `json:"file_size"`
	Quality    string    `json:"quality,omitempty"`
	Source     string    `json:"source,omitempty"`
	Resolution string    `json:"resolution,omitempty"`
	Codec      string    `json:"codec,omitempty"`
	MediaInfo  MediaInfoMap `json:"media_info,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SeriesCredit represents a cast or crew member for a series.
type SeriesCredit struct {
	ID           int    `json:"id"`
	SeriesID     string `json:"series_id"`
	PersonName   string `json:"person_name"`
	CharacterName string `json:"character_name,omitempty"`
	Role         string `json:"role"`
	ProfilePath  string `json:"profile_path,omitempty"`
	TMDBPersonID int    `json:"tmdb_person_id,omitempty"`
	DisplayOrder int    `json:"display_order"`
}

// EpisodeStats holds episode count statistics for a series.
type EpisodeStats struct {
	TotalEpisodes     int `json:"totalEpisodes"`
	DownloadedEpisodes int `json:"downloadedEpisodes"`
	MonitoredEpisodes int `json:"monitoredEpisodes"`
	MissingEpisodes   int `json:"missingEpisodes"` // monitored but not downloaded
	AiredEpisodes     int `json:"airedEpisodes"`   // episodes with air_date <= today
}

// StringSlice is a JSON-marshaled string slice for database storage.
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return json.Marshal([]string{})
	}
	return json.Marshal(s)
}

func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		str, ok := value.(string)
		if !ok {
			return nil
		}
		bytes = []byte(str)
	}
	return json.Unmarshal(bytes, s)
}

// MediaInfoMap is a JSON-marshaled map for database storage.
type MediaInfoMap map[string]interface{}

func (m MediaInfoMap) Value() (driver.Value, error) {
	return json.Marshal(m)
}

func (m *MediaInfoMap) Scan(value interface{}) error {
	if value == nil {
		*m = make(map[string]interface{})
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		str, ok := value.(string)
		if !ok {
			return nil
		}
		bytes = []byte(str)
	}
	return json.Unmarshal(bytes, m)
}

// AddSeriesRequest is the payload for adding a series via TMDB lookup.
type AddSeriesRequest struct {
	TMDBID           string `json:"tmdbId"`
	QualityProfileID string `json:"qualityProfileId"`
	RootFolderID     string `json:"rootFolderId"`
	MonitoringStatus string `json:"monitoringStatus,omitempty"`
	SeriesType       string `json:"seriesType,omitempty"`
	SeasonFolder     *bool  `json:"seasonFolder,omitempty"`
	Search           bool   `json:"search,omitempty"`
}

// UpdateSeriesRequest is the payload for updating a series.
type UpdateSeriesRequest struct {
	Title            *string  `json:"title,omitempty"`
	Year             *int     `json:"year,omitempty"`
	Overview         *string  `json:"overview,omitempty"`
	Genres           []string `json:"genres,omitempty"`
	Runtime          *int     `json:"runtime,omitempty"`
	Rating           *float64 `json:"rating,omitempty"`
	BackdropPath     *string  `json:"backdropPath,omitempty"`
	PosterPath       *string  `json:"posterPath,omitempty"`
	Network          *string  `json:"network,omitempty"`
	Status           *string  `json:"status,omitempty"`
	SeriesType       *string  `json:"seriesType,omitempty"`
	MonitoringStatus *string  `json:"monitoringStatus,omitempty"`
	QualityProfileID *string  `json:"qualityProfileId,omitempty"`
	RootFolderID     *string  `json:"rootFolderId,omitempty"`
	SeasonFolder     *bool    `json:"seasonFolder,omitempty"`
}

// SetMonitoringStatusRequest is the payload for updating monitoring status.
type SetMonitoringStatusRequest struct {
	Status string `json:"status"`
}
