package musicbrainz

// ArtistResponse is the JSON response from MusicBrainz /artist/{mbid} endpoint.
type ArtistResponse struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	SortName       string             `json:"sort-name,omitempty"`
	Disambiguation string             `json:"disambiguation,omitempty"`
	Area           *AreaResponse      `json:"area,omitempty"`
	Country        string             `json:"country,omitempty"`
	BeginArea      *AreaResponse      `json:"begin-area,omitempty"`
	EndArea        *AreaResponse      `json:"end-area,omitempty"`
	Genres         []GenreResponse    `json:"genres,omitempty"`
	Relations      []RelationResponse `json:"relations,omitempty"`
}

// ReleaseResponse is the JSON response from MusicBrainz /release/{mbid} endpoint.
type ReleaseResponse struct {
	ID           string                `json:"id"`
	Title        string                `json:"title"`
	Date         string                `json:"date,omitempty"` // ISO 8601 date (YYYY-MM-DD)
	Year         int                   `json:"year,omitempty"`
	Artists      []ArtistResponse      `json:"artist-credit,omitempty"`
	Media        []MediaResponse       `json:"media,omitempty"`
	Packaging    string                `json:"packaging,omitempty"`
	ReleaseGroup *ReleaseGroupResponse `json:"release-group,omitempty"`
	Relations    []RelationResponse    `json:"relations,omitempty"`
	Coverart     *CoverartResponse     `json:"coverart,omitempty"`
}

// RecordingResponse is the JSON response from MusicBrainz /recording/{mbid} endpoint.
type RecordingResponse struct {
	ID        string             `json:"id"`
	Title     string             `json:"title"`
	Length    int                `json:"length,omitempty"` // Duration in milliseconds
	Artists   []ArtistResponse   `json:"artist-credit,omitempty"`
	Releases  []ReleaseResponse  `json:"releases,omitempty"`
	Relations []RelationResponse `json:"relations,omitempty"`
}

// SearchResponse wraps results from a MusicBrainz search endpoint.
type SearchResponse struct {
	Count    int               `json:"count"`
	Offset   int               `json:"offset"`
	Artists  []ArtistResponse  `json:"artists,omitempty"`
	Releases []ReleaseResponse `json:"releases,omitempty"`
}

// AreaResponse represents a geographic area (country, city, etc.).
type AreaResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// GenreResponse represents a genre tag.
type GenreResponse struct {
	Count int    `json:"count"`
	Name  string `json:"name"`
}

// MediaResponse represents a physical/digital medium (CD, vinyl, etc.).
type MediaResponse struct {
	Position string          `json:"position,omitempty"`
	Title    string          `json:"title,omitempty"`
	Format   string          `json:"format,omitempty"`
	Tracks   []TrackResponse `json:"tracks,omitempty"`
}

// TrackResponse represents a track on a media.
type TrackResponse struct {
	ID        string             `json:"id"`
	Position  string             `json:"position,omitempty"`
	Number    string             `json:"number,omitempty"`
	Title     string             `json:"title,omitempty"`
	Length    int                `json:"length,omitempty"` // Duration in milliseconds
	Recording *RecordingResponse `json:"recording,omitempty"`
}

// RelationResponse represents a relationship between entities (artist-recording, etc.).
type RelationResponse struct {
	Type       string `json:"type"`
	TypeID     string `json:"type-id,omitempty"`
	Direction  string `json:"direction,omitempty"` // "forward" or "backward"
	Target     string `json:"target,omitempty"`
	TargetType string `json:"target-type,omitempty"`
}

// ReleaseGroupResponse represents a release group (album, single, etc.).
type ReleaseGroupResponse struct {
	ID    string `json:"id"`
	Type  string `json:"type,omitempty"`
	Title string `json:"title,omitempty"`
}

// CoverartResponse represents cover art information.
type CoverartResponse struct {
	FrontURL string          `json:"front,omitempty"`
	BackURL  string          `json:"back,omitempty"`
	Images   []ImageResponse `json:"images,omitempty"`
}

// ImageResponse represents a single cover art image.
type ImageResponse struct {
	URL   string   `json:"url,omitempty"`
	Front bool     `json:"front,omitempty"`
	Back  bool     `json:"back,omitempty"`
	Types []string `json:"types,omitempty"`
}

// ArtistMetadata represents the final mapped artist metadata.
type ArtistMetadata struct {
	MBID           string
	Name           string
	Disambiguation string
	Area           string
}

// ReleaseMetadata represents the final mapped release metadata.
type ReleaseMetadata struct {
	MBID        string
	Title       string
	Year        int
	Artists     []string
	Tracks      []string
	CoverartURL string
}

// RecordingMetadata represents the final mapped recording metadata.
type RecordingMetadata struct {
	MBID     string
	Title    string
	Duration int // milliseconds
	Artists  []string
}
