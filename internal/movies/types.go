package movies

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// MonitoringStatus represents whether a movie is being monitored for
// acquisition or is marked as deleted/unmonitored.
type MonitoringStatus string

const (
	MonitoringStatusMonitored   MonitoringStatus = "monitored"
	MonitoringStatusUnmonitored MonitoringStatus = "unmonitored"
	MonitoringStatusDeleted     MonitoringStatus = "deleted"
	MonitoringStatusArchived    MonitoringStatus = "archived"
)

// MovieStatus represents the current state of a movie in the library.
type MovieStatus string

const (
	MovieStatusMissing              MovieStatus = "missing"
	MovieStatusUnreleased           MovieStatus = "unreleased"
	MovieStatusDownloading          MovieStatus = "downloading"
	MovieStatusStoring              MovieStatus = "storing"
	MovieStatusAvailableWrongQuality  MovieStatus = "available_wrong_quality"
	MovieStatusAvailableRightQuality  MovieStatus = "available_right_quality"
	MovieStatusAvailableHigherQuality MovieStatus = "available_higher_quality"
)

// Movie represents a movie in the library.
type Movie struct {
	ID                 string            `json:"id"`
	Title              string            `json:"title"`
	Year               int               `json:"year,omitempty"`
	IMDBID             *string           `json:"imdb_id,omitempty"`
	TMDBID             *string           `json:"tmdb_id,omitempty"`
	TVDBID             *string           `json:"tvdb_id,omitempty"`
	Overview           string            `json:"overview,omitempty"`
	Genres             []string          `json:"genres,omitempty"`
	Runtime            int               `json:"runtime,omitempty"`
	Rating             float64           `json:"rating,omitempty"`
	BackdropPath       string            `json:"backdrop_path,omitempty"`
	PosterPath         string            `json:"poster_path,omitempty"`
	MetadataProvider   string            `json:"metadata_provider,omitempty"`
	QualityProfileID   string            `json:"quality_profile_id,omitempty"`
	LibraryID          string            `json:"library_id,omitempty"`
	Status             MovieStatus       `json:"status"`
	ReleaseDate        string            `json:"release_date,omitempty"`
	LastSearchAt       *time.Time        `json:"last_search_at,omitempty"`
	MonitoringStatus   MonitoringStatus  `json:"monitoring_status"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	DeletedAt          *time.Time        `json:"deleted_at,omitempty"`
}

// MovieFile represents a single file on disk associated with a movie.
type MovieFile struct {
	ID          string                 `json:"id"`
	MovieID     string                 `json:"movie_id"`
	FilePath    string                 `json:"file_path"`
	Size        int64                  `json:"size"`
	Quality     string                 `json:"quality,omitempty"`
	Format      string                 `json:"format,omitempty"`
	MediaInfo   map[string]interface{} `json:"media_info,omitempty"`
	FileDate    *time.Time             `json:"file_date,omitempty"`
	DateAdded   time.Time              `json:"date_added"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	DeletedAt   *time.Time             `json:"deleted_at,omitempty"`
}

// StringSlice is a JSON-marshaled string slice for database storage.
type StringSlice []string

// Value implements the driver.Valuer interface for database/sql.
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return json.Marshal([]string{})
	}
	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface.
func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, s)
}

// MediaInfoMap is a JSON-marshaled map for database storage.
type MediaInfoMap map[string]interface{}

// Value implements the driver.Valuer interface for database/sql.
func (m MediaInfoMap) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface.
func (m *MediaInfoMap) Scan(value interface{}) error {
	if value == nil {
		*m = make(map[string]interface{})
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, m)
}

// CreateMovieRequest is the payload for adding a movie.
type CreateMovieRequest struct {
	Title            string       `json:"title"`
	Year             int          `json:"year,omitempty"`
	IMDBID           *string      `json:"imdb_id,omitempty"`
	TMDBID           *string      `json:"tmdb_id,omitempty"`
	TVDBID           *string      `json:"tvdb_id,omitempty"`
	Overview         string       `json:"overview,omitempty"`
	Genres           []string     `json:"genres,omitempty"`
	Runtime          int          `json:"runtime,omitempty"`
	Rating           float64      `json:"rating,omitempty"`
	BackdropPath     string       `json:"backdrop_path,omitempty"`
	PosterPath       string       `json:"poster_path,omitempty"`
	MetadataProvider string       `json:"metadata_provider,omitempty"`
	QualityProfileID string       `json:"quality_profile_id"`
	LibraryID        string       `json:"library_id"`
	ReleaseDate      string       `json:"release_date,omitempty"`
	MonitoringStatus *string      `json:"monitoring_status,omitempty"`
	Search           bool         `json:"search,omitempty"`
}

// UpdateMovieRequest is the payload for updating a movie.
type UpdateMovieRequest struct {
	Title            *string      `json:"title,omitempty"`
	Year             *int         `json:"year,omitempty"`
	Overview         *string      `json:"overview,omitempty"`
	Genres           []string     `json:"genres,omitempty"`
	Runtime          *int         `json:"runtime,omitempty"`
	Rating           *float64     `json:"rating,omitempty"`
	BackdropPath     *string      `json:"backdrop_path,omitempty"`
	PosterPath       *string      `json:"poster_path,omitempty"`
	MonitoringStatus *string      `json:"monitoring_status,omitempty"`
	QualityProfileID *string      `json:"quality_profile_id,omitempty"`
	LibraryID        *string      `json:"library_id,omitempty"`
}

// ListMoviesFilter is used to filter the movies list.
type ListMoviesFilter struct {
	MonitoringStatus *MonitoringStatus `json:"monitoring_status,omitempty"`
	Year             *int              `json:"year,omitempty"`
	MinRating        *float64          `json:"min_rating,omitempty"`
	Genre            *string           `json:"genre,omitempty"`
}

// ListMoviesResponse wraps paginated movie results.
type ListMoviesResponse struct {
	Data  []Movie `json:"data"`
	Total int     `json:"total"`
	Page  int     `json:"page"`
	Limit int     `json:"limit"`
}

// SearchMoviesResponse wraps search results.
type SearchMoviesResponse struct {
	Data []Movie `json:"data"`
}

// SetMonitoringStatusRequest is the payload for updating monitoring status.
type SetMonitoringStatusRequest struct {
	Status string `json:"status"`
}

// MovieAddedEvent is emitted when a movie is added to the library.
type MovieAddedEvent struct {
	MovieID   string    `json:"movie_id"`
	Title     string    `json:"title"`
	AddedAt   time.Time `json:"added_at"`
}

// Topic returns the event topic.
func (e *MovieAddedEvent) Topic() string { return TopicMovieAdded }

// MovieUpdatedEvent is emitted when a movie is updated.
type MovieUpdatedEvent struct {
	MovieID    string    `json:"movie_id"`
	FieldsSet  []string  `json:"fields_set"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Topic returns the event topic.
func (e *MovieUpdatedEvent) Topic() string { return TopicMovieUpdated }

// MovieDeletedEvent is emitted when a movie is deleted.
type MovieDeletedEvent struct {
	MovieID   string    `json:"movie_id"`
	DeletedAt time.Time `json:"deleted_at"`
}

// Topic returns the event topic.
func (e *MovieDeletedEvent) Topic() string { return TopicMovieDeleted }

// MonitoringChangedEvent is emitted when monitoring status changes.
type MonitoringChangedEvent struct {
	MovieID  string           `json:"movie_id"`
	OldStatus MonitoringStatus `json:"old_status"`
	NewStatus MonitoringStatus `json:"new_status"`
	ChangedAt time.Time        `json:"changed_at"`
}

// Topic returns the event topic.
func (e *MonitoringChangedEvent) Topic() string { return TopicMonitoringChanged }

// Size-limit mode constants for QualityDefinition.
const (
	// SizeModeAbsolute means MinFileSize/MaxFileSize are raw byte counts.
	SizeModeAbsolute = "absolute"
	// SizeModePerMinute means MinFileSize/MaxFileSize are megabytes per
	// minute of runtime (matching Radarr/Sonarr behaviour).
	SizeModePerMinute = "per_minute"
)

// DefaultRuntimeMinutes is used when a movie/episode has no runtime info
// and the quality definition uses per-minute sizing.
const DefaultRuntimeMinutes = 120

// QualityDefinition represents a single quality tier (resolution, source, codec).
type QualityDefinition struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Title        string    `json:"title,omitempty"`
	Source       string    `json:"source"`        // e.g., "BluRay", "HDTV", "WebRip"
	Resolution   string    `json:"resolution"`    // e.g., "1080p", "720p", "2160p"
	Modifier     string    `json:"modifier,omitempty"` // e.g., "REMUX", "PROPER"
	SizeMode     string    `json:"size_mode"`     // "absolute" or "per_minute"
	MinFileSize  int64     `json:"min_file_size,omitempty"` // bytes (absolute) or MB/min (per_minute)
	MaxFileSize  int64     `json:"max_file_size,omitempty"` // bytes (absolute) or MB/min (per_minute); 0 = unlimited
	PreferredAt  int       `json:"preferred_at"`  // order preference (lower = better)
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

// EffectiveSizeLimits returns the min/max size in bytes for a given
// media runtime (in minutes). When SizeMode is "absolute" the runtime
// is ignored and raw byte values are returned as-is.
func (qd *QualityDefinition) EffectiveSizeLimits(runtimeMinutes int) (minBytes, maxBytes int64) {
	if qd.SizeMode != SizeModePerMinute {
		return qd.MinFileSize, qd.MaxFileSize
	}
	if runtimeMinutes <= 0 {
		runtimeMinutes = DefaultRuntimeMinutes
	}
	const mb = int64(1024 * 1024)
	minBytes = qd.MinFileSize * mb * int64(runtimeMinutes)
	maxBytes = qd.MaxFileSize * mb * int64(runtimeMinutes)
	if qd.MaxFileSize == 0 {
		maxBytes = 0 // preserve 0 = unlimited
	}
	return minBytes, maxBytes
}

// QualityProfileItem represents a quality within a quality profile.
type QualityProfileItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Preferred bool   `json:"preferred"`
	Allowed  bool   `json:"allowed"`
}

// QualityProfile represents a named collection of quality tiers with preferences.
type QualityProfile struct {
	ID                string                  `json:"id"`
	Name              string                  `json:"name"`
	UpgradeAllowed    bool                    `json:"upgrade_allowed"`
	Cutoff            string                  `json:"cutoff"`                   // quality definition ID
	Language          string                  `json:"language,omitempty"`        // ISO 639-1 code
	FormatItems       []CustomFormatScore     `json:"format_items,omitempty"`   // custom format scores
	Items             []QualityProfileItem    `json:"items"`                    // quality definitions
	MinFormatScore    int                     `json:"min_format_score,omitempty"`
	CutoffFormatScore int                     `json:"cutoff_format_score,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
	DeletedAt         *time.Time              `json:"deleted_at,omitempty"`
}

// CreateQualityDefinitionRequest is the payload for adding a quality definition.
type CreateQualityDefinitionRequest struct {
	Name         string `json:"name"`
	Title        string `json:"title,omitempty"`
	Source       string `json:"source"`
	Resolution   string `json:"resolution"`
	Modifier     string `json:"modifier,omitempty"`
	SizeMode     string `json:"size_mode,omitempty"`    // "absolute" (default) or "per_minute"
	MinFileSize  int64  `json:"min_file_size,omitempty"`
	MaxFileSize  int64  `json:"max_file_size,omitempty"`
	PreferredAt  int    `json:"preferred_at,omitempty"`
}

// UpdateQualityDefinitionRequest is the payload for updating a quality definition.
type UpdateQualityDefinitionRequest struct {
	Name        *string `json:"name,omitempty"`
	Title       *string `json:"title,omitempty"`
	Source      *string `json:"source,omitempty"`
	Resolution  *string `json:"resolution,omitempty"`
	Modifier    *string `json:"modifier,omitempty"`
	SizeMode    *string `json:"size_mode,omitempty"`
	MinFileSize *int64  `json:"min_file_size,omitempty"`
	MaxFileSize *int64  `json:"max_file_size,omitempty"`
	PreferredAt *int    `json:"preferred_at,omitempty"`
}

// CreateQualityProfileRequest is the payload for adding a quality profile.
type CreateQualityProfileRequest struct {
	Name               string                   `json:"name"`
	UpgradeAllowed     bool                     `json:"upgrade_allowed"`
	Cutoff             string                   `json:"cutoff"`
	Language           string                   `json:"language,omitempty"`
	Items              []QualityProfileItem     `json:"items"`
	MinFormatScore     int                      `json:"min_format_score,omitempty"`
	CutoffFormatScore  int                      `json:"cutoff_format_score,omitempty"`
}

// UpdateQualityProfileRequest is the payload for updating a quality profile.
type UpdateQualityProfileRequest struct {
	Name               *string                  `json:"name,omitempty"`
	UpgradeAllowed     *bool                    `json:"upgrade_allowed,omitempty"`
	Cutoff             *string                  `json:"cutoff,omitempty"`
	Language           *string                  `json:"language,omitempty"`
	Items              []QualityProfileItem     `json:"items,omitempty"`
	MinFormatScore     *int                     `json:"min_format_score,omitempty"`
	CutoffFormatScore  *int                     `json:"cutoff_format_score,omitempty"`
}

// CustomFormatScore represents the score assigned to a custom format within a quality profile.
type CustomFormatScore struct {
	CustomFormatID string `json:"custom_format_id"`
	Score          int    `json:"score"` // positive or negative score
}

// CustomFormatFilterCondition is the type of condition used in a custom format filter.
type CustomFormatFilterCondition string

const (
	// Exact match
	ConditionEquals CustomFormatFilterCondition = "equals"
	// Regex pattern match
	ConditionRegex CustomFormatFilterCondition = "regex"
	// Numeric range (value should be "min,max" or "min," or ",max")
	ConditionRange CustomFormatFilterCondition = "range"
	// Member of a list (value should be comma-separated)
	ConditionIn CustomFormatFilterCondition = "in"
	// Numeric comparison operators
	ConditionGreaterThan CustomFormatFilterCondition = "gt"
	ConditionGreaterThanOrEqual CustomFormatFilterCondition = "gte"
	ConditionLessThan CustomFormatFilterCondition = "lt"
	ConditionLessThanOrEqual CustomFormatFilterCondition = "lte"
)

// CustomFormatFilter represents a single filter condition within a custom format.
type CustomFormatFilter struct {
	ID                 string                         `json:"id"`
	CustomFormatID     string                         `json:"custom_format_id"`
	Field              string                         `json:"field"`           // codec, source, year, bitdepth, etc.
	Condition          CustomFormatFilterCondition    `json:"condition"`       // equals, regex, range, in, gt, gte, lt, lte
	Value              string                         `json:"value"`           // field-specific value
	Order              int                            `json:"order"`           // display order
	CreatedAt          time.Time                      `json:"created_at"`
	UpdatedAt          time.Time                      `json:"updated_at"`
}

// CustomFormat represents a named set of filters and tags for scoring releases.
type CustomFormat struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Tags        []string                  `json:"tags,omitempty"`           // user-defined tags (e.g., "hdr", "anime", "4k")
	Filters     []CustomFormatFilter      `json:"filters"`                  // all filters use implicit AND logic
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
	DeletedAt   *time.Time                `json:"deleted_at,omitempty"`
}

// CreateCustomFormatRequest is the payload for adding a custom format.
type CreateCustomFormatRequest struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	Tags        []string                 `json:"tags,omitempty"`
	Filters     []CreateCustomFormatFilterRequest `json:"filters"`
}

// CreateCustomFormatFilterRequest is the payload for a filter within a custom format creation.
type CreateCustomFormatFilterRequest struct {
	Field     string                         `json:"field"`
	Condition CustomFormatFilterCondition    `json:"condition"`
	Value     string                         `json:"value"`
	Order     int                            `json:"order,omitempty"`
}

// UpdateCustomFormatRequest is the payload for updating a custom format.
type UpdateCustomFormatRequest struct {
	Name        *string                  `json:"name,omitempty"`
	Description *string                  `json:"description,omitempty"`
	Tags        []string                 `json:"tags,omitempty"`
	Filters     []CreateCustomFormatFilterRequest `json:"filters,omitempty"`
}

// HistoryEntry represents a combined import or search history record.
type HistoryEntry struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // "import" or "search"
	Status    string    `json:"status"`
	Title     string    `json:"title,omitempty"`
	Error     string    `json:"error,omitempty"`
	SourcePath string   `json:"sourcePath,omitempty"`
	DestPath   string   `json:"destPath,omitempty"`
	Date      time.Time `json:"date"`
}
