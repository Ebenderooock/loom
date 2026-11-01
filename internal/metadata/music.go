package metadata

import (
	"context"
	"time"
)

// ArtistMetadata represents metadata for a music artist from any provider
// (currently MusicBrainz). The MBID is the MusicBrainz identifier.
type ArtistMetadata struct {
	MBID           string    `json:"mbid"`
	Name           string    `json:"name"`
	SortName       string    `json:"sort_name,omitempty"`
	Disambiguation string    `json:"disambiguation,omitempty"`
	Type           string    `json:"type,omitempty"` // Person, Group, Orchestra, Choir, ...
	Country        string    `json:"country,omitempty"`
	Overview       string    `json:"overview,omitempty"`
	Genres         []string  `json:"genres,omitempty"`
	ImageURL       string    `json:"image_url,omitempty"`
	CachedAt       time.Time `json:"cached_at,omitempty"`

	BeginArea   string         `json:"begin_area,omitempty"`
	BeginDate   string         `json:"begin_date,omitempty"`
	EndDate     string         `json:"end_date,omitempty"`
	Ended       bool           `json:"ended,omitempty"`
	Rating      float64        `json:"rating,omitempty"` // 0-5
	RatingVotes int            `json:"rating_votes,omitempty"`
	Links       []ExternalLink `json:"links,omitempty"`
}

// ExternalLink is a curated link to a service page for an artist or album,
// such as Spotify, Apple Music, YouTube Music, Wikipedia or the official site.
type ExternalLink struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// AlbumMetadata represents metadata for an album. An "album" maps to a
// MusicBrainz release-group (the abstract album), not a specific edition.
type AlbumMetadata struct {
	MBID           string                `json:"mbid"` // release-group MBID
	Title          string                `json:"title"`
	ArtistMBID     string                `json:"artist_mbid,omitempty"`
	ArtistName     string                `json:"artist_name,omitempty"`
	Artists        []AlbumArtistMetadata `json:"artists,omitempty"`
	Type           string                `json:"type,omitempty"`            // Album, EP, Single, Broadcast, Other
	SecondaryTypes []string              `json:"secondary_types,omitempty"` // Live, Compilation, Soundtrack, ...
	ReleaseDate    string                `json:"release_date,omitempty"`    // first release date (ISO 8601)
	Genres         []string              `json:"genres,omitempty"`
	CoverArtURL    string                `json:"cover_art_url,omitempty"`
	CachedAt       time.Time             `json:"cached_at,omitempty"`

	Rating      float64        `json:"rating,omitempty"` // 0-5
	RatingVotes int            `json:"rating_votes,omitempty"`
	Links       []ExternalLink `json:"links,omitempty"`
}

// AlbumArtistMetadata preserves the full credited artist list on a release
// group. ArtistMBID is empty when MusicBrainz has no linked artist entity.
type AlbumArtistMetadata struct {
	ArtistMBID string `json:"artist_mbid,omitempty"`
	Name       string `json:"name"`
	JoinPhrase string `json:"join_phrase,omitempty"`
}

// AlbumReleaseMetadata represents a specific edition (MusicBrainz release) of an
// album. This carries the concrete track list used for acquisition matching and
// completeness tracking.
type AlbumReleaseMetadata struct {
	MBID           string          `json:"mbid"` // release MBID
	Title          string          `json:"title"`
	Status         string          `json:"status,omitempty"` // Official, Promotion, Bootleg, ...
	Disambiguation string          `json:"disambiguation,omitempty"`
	ReleaseDate    string          `json:"release_date,omitempty"`
	Country        string          `json:"country,omitempty"`
	Label          string          `json:"label,omitempty"`
	Format         string          `json:"format,omitempty"` // CD, Digital Media, Vinyl, ...
	MediaCount     int             `json:"media_count,omitempty"`
	TrackCount     int             `json:"track_count,omitempty"`
	Tracks         []TrackMetadata `json:"tracks,omitempty"`
}

// TrackMetadata represents a single track on a release.
type TrackMetadata struct {
	MBID        string `json:"mbid"`     // recording MBID (can repeat across releases)
	TrackID     string `json:"track_id"` // release-track MBID (unique on a release)
	Title       string `json:"title"`
	TrackNumber int    `json:"track_number"`
	DiscNumber  int    `json:"disc_number"`
	DurationMs  int    `json:"duration_ms,omitempty"`
	ArtistName  string `json:"artist_name,omitempty"`
}

// MusicMetadataProvider is implemented by music metadata sources (MusicBrainz).
// It is intentionally separate from MetadataProvider so video providers
// (TMDB/TVDB) are not forced to implement music lookups.
type MusicMetadataProvider interface {
	// Name returns the provider identifier (e.g. "musicbrainz").
	Name() string

	// SearchArtist searches for artists by free-text query.
	SearchArtist(ctx context.Context, query string, limit int) ([]*ArtistMetadata, error)

	// SearchAlbum searches release-groups (not concrete release editions).
	SearchAlbum(ctx context.Context, query string, limit int) ([]*AlbumMetadata, error)

	// GetArtist fetches a single artist by MBID.
	GetArtist(ctx context.Context, mbid string) (*ArtistMetadata, error)

	// GetArtistAlbums returns the artist's studio albums, EPs and singles that
	// have an official release, deduplicated by release-group MBID.
	GetArtistAlbums(ctx context.Context, artistMBID string) ([]*AlbumMetadata, error)

	// GetAlbum fetches an album (release-group) and its candidate editions
	// (releases). The releases carry no track lists; use GetAlbumRelease for
	// the full track listing of a chosen edition.
	GetAlbum(ctx context.Context, releaseGroupMBID string) (*AlbumMetadata, []*AlbumReleaseMetadata, error)

	// GetAlbumRelease fetches a single edition (release) including its tracks.
	GetAlbumRelease(ctx context.Context, releaseMBID string) (*AlbumReleaseMetadata, error)
}

// ArtistArtwork is artwork and descriptive text for an artist from an artwork
// source (fanart.tv, TheAudioDB). AlbumCovers maps release-group MBIDs to cover
// URLs when the source returns them with the artist.
type ArtistArtwork struct {
	ThumbURL      string            `json:"thumb_url,omitempty"`
	BackgroundURL string            `json:"background_url,omitempty"`
	LogoURL       string            `json:"logo_url,omitempty"`
	Biography     string            `json:"biography,omitempty"`
	AlbumCovers   map[string]string `json:"album_covers,omitempty"`
}

// AlbumArtwork is artwork and descriptive text for an album (release-group).
type AlbumArtwork struct {
	CoverURL    string `json:"cover_url,omitempty"`
	Description string `json:"description,omitempty"`
}

// MusicArtworkProvider supplies artwork that MusicBrainz does not carry. All
// lookups are best-effort: a nil result with a nil error means no artwork.
type MusicArtworkProvider interface {
	ArtistArtwork(ctx context.Context, artistMBID string) (*ArtistArtwork, error)
	AlbumArtwork(ctx context.Context, releaseGroupMBID string) (*AlbumArtwork, error)
}
