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
	LastSearchAt       *time.Time        `json:"last_search_at,omitempty"`
	MonitoringStatus   MonitoringStatus  `json:"monitoring_status"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	DeletedAt          *time.Time        `json:"deleted_at,omitempty"`
}

// RootFolder represents a filesystem path where movies are stored.
type RootFolder struct {
	ID            string     `json:"id"`
	Path          string     `json:"path"`
	FreeSpace     int64      `json:"free_space"`
	UnmappedCount int        `json:"unmapped_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
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
	MonitoringStatus *string      `json:"monitoring_status,omitempty"`
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

// CreateRootFolderRequest is the payload for adding a root folder.
type CreateRootFolderRequest struct {
	Path string `json:"path"`
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
