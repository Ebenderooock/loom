package tvdb

// SeriesResponse is the JSON response from TVDB /series/{id} endpoint.
type SeriesResponse struct {
	Data SeriesData `json:"data"`
}

// SeriesData contains the series metadata from TVDB.
type SeriesData struct {
	ID           int        `json:"id"`
	Name         string     `json:"name"`
	Overview     string     `json:"overview"`
	Image        string     `json:"image"`
	FirstAirDate string     `json:"firstAirDate"`
	Status       StatusInfo `json:"status"`
	Year         string     `json:"year"` // TVDB returns as string
	ExternalIDs  IDsInfo    `json:"externalIds"`
	SeriesCount  int        `json:"seriesCount,omitempty"` // Used in some responses
	Characters   []interface{} `json:"characters,omitempty"`
}

// StatusInfo contains series status.
type StatusInfo struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// IDsInfo contains external IDs (IMDB, TVDB, etc.).
type IDsInfo struct {
	IMDB string `json:"imdb,omitempty"`
	TVDb int    `json:"tvdb,omitempty"`
}

// EpisodeResponse is the JSON response from TVDB /episodes/{id} endpoint.
type EpisodeResponse struct {
	Data EpisodeData `json:"data"`
}

// EpisodeData contains episode metadata.
type EpisodeData struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	Image         string `json:"image"`
	AirDate       string `json:"airDate"`
	Runtime       int    `json:"runtime,omitempty"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	Ratings       RatingsInfo `json:"ratings,omitempty"`
	ExternalIDs   IDsInfo `json:"externalIds,omitempty"`
}

// RatingsInfo contains rating data.
type RatingsInfo struct {
	IMDB float64 `json:"imdb,omitempty"`
}

// SearchResponse wraps search results from TVDB.
type SearchResponse struct {
	Data []SearchResult `json:"data"`
	Meta SearchMeta     `json:"meta,omitempty"`
}

// SearchResult is a single search result.
type SearchResult struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	FirstAirDate string `json:"firstAirDate,omitempty"`
	Overview     string `json:"overview,omitempty"`
	Image        string `json:"image,omitempty"`
	Type         string `json:"type,omitempty"` // "series", "movie", etc.
	ExternalIDs  IDsInfo `json:"externalIds,omitempty"`
}

// SearchMeta contains pagination info.
type SearchMeta struct {
	Page      int `json:"page,omitempty"`
	PageSize  int `json:"pageSize,omitempty"`
	TotalSize int `json:"totalSize,omitempty"`
}

// LoginRequest is sent to TVDB /login endpoint.
type LoginRequest struct {
	APIKey string `json:"apikey"`
	PIN    string `json:"pin,omitempty"`
}

// LoginResponse contains the JWT token from TVDB /login.
type LoginResponse struct {
	Data LoginData `json:"data"`
}

// LoginData contains the JWT token.
type LoginData struct {
	Token string `json:"token"`
}

// ErrorResponse is the standard TVDB API error response.
type ErrorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}
