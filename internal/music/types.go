package music

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// MonitoringStatus represents whether an artist is actively monitored.
type MonitoringStatus string

const (
	MonitoringMonitored   MonitoringStatus = "monitored"
	MonitoringUnmonitored MonitoringStatus = "unmonitored"
)

// Artist represents a managed music artist (MusicBrainz artist MBID).
type Artist struct {
	ID                string           `json:"id"`
	MBID              string           `json:"mbid"`
	Name              string           `json:"name"`
	SortName          string           `json:"sort_name,omitempty"`
	Disambiguation    string           `json:"disambiguation,omitempty"`
	ArtistType        string           `json:"artist_type,omitempty"`
	Country           string           `json:"country,omitempty"`
	Overview          string           `json:"overview,omitempty"`
	Genres            StringSlice      `json:"genres,omitempty"`
	ImageURL          string           `json:"image_url,omitempty"`
	Path              string           `json:"path,omitempty"`
	LibraryID         string           `json:"library_id,omitempty"`
	QualityProfileID  string           `json:"quality_profile_id,omitempty"`
	MetadataProfileID string           `json:"metadata_profile_id,omitempty"`
	MonitoringStatus  MonitoringStatus `json:"monitoring_status"`
	MetadataProvider  string           `json:"metadata_provider,omitempty"`
	LastSearchAt      *time.Time       `json:"last_search_at,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`

	// Populated on read, not stored directly.
	Albums []*Album     `json:"albums,omitempty"`
	Stats  *ArtistStats `json:"stats,omitempty"`
}

// Album represents an abstract album (MusicBrainz release-group) for an artist.
type Album struct {
	ID                string      `json:"id"`
	MBID              string      `json:"mbid"`
	ArtistID          string      `json:"artist_id"`
	Title             string      `json:"title"`
	AlbumType         string      `json:"album_type,omitempty"`
	SecondaryTypes    StringSlice `json:"secondary_types,omitempty"`
	ReleaseDate       string      `json:"release_date,omitempty"`
	Genres            StringSlice `json:"genres,omitempty"`
	CoverArtURL       string      `json:"cover_art_url,omitempty"`
	Overview          string      `json:"overview,omitempty"`
	Monitored         bool        `json:"monitored"`
	SelectedReleaseID string      `json:"selected_release_id,omitempty"`
	LastSearchAt      *time.Time  `json:"last_search_at,omitempty"`
	ReleasesFetchedAt *time.Time  `json:"releases_fetched_at,omitempty"`
	TracksFetchedAt   *time.Time  `json:"tracks_fetched_at,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`

	// Populated on read, not stored directly.
	Releases []*AlbumRelease `json:"releases,omitempty"`
	Tracks   []*Track        `json:"tracks,omitempty"`
}

// AlbumRelease represents a concrete edition (MusicBrainz release) of an album.
type AlbumRelease struct {
	ID             string    `json:"id"`
	MBID           string    `json:"mbid"`
	AlbumID        string    `json:"album_id"`
	Title          string    `json:"title,omitempty"`
	Disambiguation string    `json:"disambiguation,omitempty"`
	Status         string    `json:"status,omitempty"`
	ReleaseDate    string    `json:"release_date,omitempty"`
	Country        string    `json:"country,omitempty"`
	Label          string    `json:"label,omitempty"`
	Format         string    `json:"format,omitempty"`
	MediaCount     int       `json:"media_count"`
	TrackCount     int       `json:"track_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Track represents a single track on a selected album release.
type Track struct {
	ID            string    `json:"id"`
	RecordingMBID string    `json:"recording_mbid,omitempty"`
	TrackMBID     string    `json:"track_mbid,omitempty"`
	AlbumID       string    `json:"album_id"`
	ReleaseID     string    `json:"release_id,omitempty"`
	Title         string    `json:"title,omitempty"`
	TrackNumber   int       `json:"track_number"`
	DiscNumber    int       `json:"disc_number"`
	DurationMs    int       `json:"duration_ms,omitempty"`
	ArtistName    string    `json:"artist_name,omitempty"`
	Monitored     bool      `json:"monitored"`
	HasFile       bool      `json:"has_file"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TrackFile represents a physical audio file linked to a track.
type TrackFile struct {
	ID        string       `json:"id"`
	TrackID   string       `json:"track_id,omitempty"`
	AlbumID   string       `json:"album_id,omitempty"`
	ArtistID  string       `json:"artist_id,omitempty"`
	FilePath  string       `json:"file_path"`
	Size      int64        `json:"size"`
	Quality   string       `json:"quality,omitempty"`
	Format    string       `json:"format,omitempty"`
	Bitrate   int          `json:"bitrate,omitempty"`
	MediaInfo MediaInfoMap `json:"media_info,omitempty"`
	FileDate  *time.Time   `json:"file_date,omitempty"`
	DateAdded time.Time    `json:"date_added"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// AudioQualityDefinition is an objective audio quality tier (format/bitrate).
type AudioQualityDefinition struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Format    string `json:"format,omitempty"`
	Bitrate   int    `json:"bitrate,omitempty"`
	VBR       bool   `json:"vbr"`
	Lossless  bool   `json:"lossless"`
	TierOrder int    `json:"tier_order"`
}

// AudioQualityProfile defines allowed/preferred audio qualities and a cutoff.
type AudioQualityProfile struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Items          json.RawMessage   `json:"items"`
	Cutoff         string            `json:"cutoff,omitempty"`
	UpgradeAllowed bool              `json:"upgrade_allowed"`
	FormatItems    []AudioFormatItem `json:"format_items"`
	MinFormatScore int               `json:"min_format_score"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// AudioFormatItem ties a custom format to a score within an audio quality
// profile. Releases accumulate the scores of every custom format they match;
// the total contributes to ranking and is gated by MinFormatScore.
type AudioFormatItem struct {
	FormatID string `json:"format_id"`
	Score    int    `json:"score"`
}

// UpdateAudioQualityProfileRequest patches an audio quality profile's
// custom-format scoring and acquisition policy. Nil fields are left unchanged.
type UpdateAudioQualityProfileRequest struct {
	Cutoff         *string           `json:"cutoff,omitempty"`
	UpgradeAllowed *bool             `json:"upgrade_allowed,omitempty"`
	FormatItems    []AudioFormatItem `json:"format_items,omitempty"`
	MinFormatScore *int              `json:"min_format_score,omitempty"`
}

// MetadataProfile controls which album/release types are monitored.
type MetadataProfile struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	PrimaryTypes    StringSlice `json:"primary_types"`
	SecondaryTypes  StringSlice `json:"secondary_types"`
	ReleaseStatuses StringSlice `json:"release_statuses"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// ArtistStats holds rollup counts for an artist.
type ArtistStats struct {
	AlbumCount          int `json:"albumCount"`
	MonitoredAlbumCount int `json:"monitoredAlbumCount"`
	TrackCount          int `json:"trackCount"`
	TrackFileCount      int `json:"trackFileCount"`
	MissingTrackCount   int `json:"missingTrackCount"` // monitored tracks without a file
}

// AddArtistRequest is the payload for adding an artist via MusicBrainz lookup.
type AddArtistRequest struct {
	MBID              string `json:"mbid"`
	QualityProfileID  string `json:"qualityProfileId"`
	LibraryID         string `json:"libraryId"`
	MetadataProfileID string `json:"metadataProfileId,omitempty"`
	MonitoringStatus  string `json:"monitoringStatus,omitempty"`
	Search            bool   `json:"search,omitempty"`
}

// UpdateArtistRequest is the payload for updating an artist.
type UpdateArtistRequest struct {
	MonitoringStatus  *string `json:"monitoringStatus,omitempty"`
	QualityProfileID  *string `json:"qualityProfileId,omitempty"`
	LibraryID         *string `json:"libraryId,omitempty"`
	MetadataProfileID *string `json:"metadataProfileId,omitempty"`
}

// SetMonitoringRequest sets an artist's monitoring status.
type SetMonitoringRequest struct {
	Status string `json:"status"`
}

// SetAlbumMonitoredRequest toggles album monitoring.
type SetAlbumMonitoredRequest struct {
	Monitored bool `json:"monitored"`
}

// ArtistLookupResult is a metadata search hit (not yet in the library).
type ArtistLookupResult struct {
	MBID           string   `json:"mbid"`
	Name           string   `json:"name"`
	Disambiguation string   `json:"disambiguation,omitempty"`
	Type           string   `json:"type,omitempty"`
	Country        string   `json:"country,omitempty"`
	Genres         []string `json:"genres,omitempty"`
	ImageURL       string   `json:"image_url,omitempty"`
	AlreadyAdded   bool     `json:"already_added"`
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
	if len(bytes) == 0 {
		*s = []string{}
		return nil
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
	if len(bytes) == 0 {
		*m = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal(bytes, m)
}
