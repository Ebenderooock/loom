package metadata

import (
	"context"
	"time"
)

// MovieMetadata represents metadata for a movie from any provider.
type MovieMetadata struct {
	TMDBID      *string   `json:"tmdb_id,omitempty"`
	IMDBID      *string   `json:"imdb_id,omitempty"`
	TVDBID      *string   `json:"tvdb_id,omitempty"`
	Title       string    `json:"title"`
	Year        int       `json:"year,omitempty"`
	Overview    string    `json:"overview,omitempty"`
	PosterPath  string    `json:"poster_path,omitempty"`
	ReleaseDate string    `json:"release_date,omitempty"` // ISO 8601
	Runtime     int       `json:"runtime,omitempty"`
	Genres      []string  `json:"genres,omitempty"`
	Rating      float64   `json:"rating,omitempty"`
	CachedAt    time.Time `json:"cached_at,omitempty"`
}

// SeriesMetadata represents metadata for a TV series from any provider.
type SeriesMetadata struct {
	TMDBID       *string   `json:"tmdb_id,omitempty"`
	IMDBID       *string   `json:"imdb_id,omitempty"`
	TVDBID       *string   `json:"tvdb_id,omitempty"`
	Title        string    `json:"title"`
	Overview     string    `json:"overview,omitempty"`
	PosterPath   string    `json:"poster_path,omitempty"`
	FirstAirDate string    `json:"first_air_date,omitempty"` // ISO 8601
	Genres       []string  `json:"genres,omitempty"`
	Rating       float64   `json:"rating,omitempty"`
	Seasons      int       `json:"seasons,omitempty"`
	CachedAt     time.Time `json:"cached_at,omitempty"`
}

// EpisodeMetadata represents metadata for a single episode.
type EpisodeMetadata struct {
	TVDBID    *string   `json:"tvdb_id,omitempty"`
	TMDBID    *string   `json:"tmdb_id,omitempty"`
	Season    int       `json:"season"`
	Episode   int       `json:"episode"`
	Title     string    `json:"title"`
	Overview  string    `json:"overview,omitempty"`
	AirDate   string    `json:"air_date,omitempty"` // ISO 8601
	Runtime   int       `json:"runtime,omitempty"`
	Rating    float64   `json:"rating,omitempty"`
	CachedAt  time.Time `json:"cached_at,omitempty"`
}

// MetadataProvider is the interface implemented by metadata sources
// (TMDB, TVDB, MusicBrainz).
type MetadataProvider interface {
	// Name returns the provider's identifier (e.g. "tmdb", "tvdb", "musicbrainz").
	Name() string

	// FindMovie searches for a movie by title+year or external IDs.
	// Partial results are acceptable (some fields may be empty).
	FindMovie(ctx context.Context, title string, year int, externalIDs map[string]string) ([]*MovieMetadata, error)

	// FindSeries searches for a series by title or external IDs.
	FindSeries(ctx context.Context, title string, externalIDs map[string]string) ([]*SeriesMetadata, error)

	// FindEpisode searches for an episode by series ID and season/episode numbers.
	FindEpisode(ctx context.Context, seriesID string, season int, episode int) (*EpisodeMetadata, error)
}

// SearchMovieParams carries the search criteria for FindMovie.
type SearchMovieParams struct {
	Title   string
	Year    int
	IMDBID  string
	TMDBID  string
	TVDBID  string
}

// SearchSeriesParams carries the search criteria for FindSeries.
type SearchSeriesParams struct {
	Title  string
	IMDBID string
	TMDBID string
	TVDBID string
}
