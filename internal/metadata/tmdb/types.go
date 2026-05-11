package tmdb

// MovieResponse is the JSON response from TMDb /movie/{id} endpoint.
type MovieResponse struct {
	ID              int    `json:"id"`
	IMDBID          string `json:"imdb_id"`
	Title           string `json:"title"`
	ReleaseDate     string `json:"release_date"`
	Overview        string `json:"overview"`
	PosterPath      string `json:"poster_path"`
	Runtime         int    `json:"runtime"`
	VoteAverage     float64 `json:"vote_average"`
	Genres          []GenreResponse `json:"genres"`
	ExternalIDs     *ExternalIDResponse `json:"external_ids"`
}

// TVResponse is the JSON response from TMDb /tv/{id} endpoint.
type TVResponse struct {
	ID              int    `json:"id"`
	IMDBID          string `json:"imdb_id"`
	Name            string `json:"name"`
	FirstAirDate    string `json:"first_air_date"`
	Overview        string `json:"overview"`
	PosterPath      string `json:"poster_path"`
	VoteAverage     float64 `json:"vote_average"`
	NumberOfSeasons int    `json:"number_of_seasons"`
	Genres          []GenreResponse `json:"genres"`
	ExternalIDs     *ExternalIDResponse `json:"external_ids"`
}

// EpisodeResponse is the JSON response from TMDb /tv/{id}/season/{season}/episode/{episode} endpoint.
type EpisodeResponse struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Overview        string `json:"overview"`
	AirDate         string `json:"air_date"`
	EpisodeNumber   int    `json:"episode_number"`
	SeasonNumber    int    `json:"season_number"`
	Runtime         int    `json:"runtime"`
	VoteAverage     float64 `json:"vote_average"`
	ExternalIDs     *ExternalIDResponse `json:"external_ids"`
}

// SearchResponse wraps the results from a TMDb search endpoint.
type SearchResponse struct {
	Results     []SearchResult `json:"results"`
	TotalPages  int            `json:"total_pages"`
	TotalResults int           `json:"total_results"`
	Page        int            `json:"page"`
}

// SearchResult is a single result in a search response.
type SearchResult struct {
	ID              int     `json:"id"`
	Title           string  `json:"title,omitempty"`
	Name            string  `json:"name,omitempty"`
	ReleaseDate     string  `json:"release_date,omitempty"`
	FirstAirDate    string  `json:"first_air_date,omitempty"`
	Overview        string  `json:"overview"`
	PosterPath      string  `json:"poster_path"`
	VoteAverage     float64 `json:"vote_average"`
	MediaType       string  `json:"media_type"`
}

// GenreResponse is a genre object in TMDb responses.
type GenreResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ExternalIDResponse contains external IDs (IMDB, TVDB, etc.) from TMDb.
type ExternalIDResponse struct {
	IMDBID string `json:"imdb_id"`
	TVDBID int    `json:"tvdb_id"`
	TMDBID int    `json:"id"`
}

// ErrorResponse is the standard TMDb API error response.
type ErrorResponse struct {
	StatusCode int    `json:"status_code"`
	StatusMessage string `json:"status_message"`
	Success    bool   `json:"success"`
}

// CreditsResponse wraps the cast and crew from TMDb /movie/{id}/credits.
type CreditsResponse struct {
	ID   int          `json:"id"`
	Cast []CastEntry  `json:"cast"`
	Crew []CrewEntry  `json:"crew"`
}

// CastEntry is a single cast member in a credits response.
type CastEntry struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Character   string  `json:"character"`
	ProfilePath string  `json:"profile_path"`
	Order       int     `json:"order"`
	KnownFor    string  `json:"known_for_department"`
}

// CrewEntry is a single crew member in a credits response.
type CrewEntry struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Job         string `json:"job"`
	Department  string `json:"department"`
	ProfilePath string `json:"profile_path"`
}

// ReleaseDatesResponse wraps the /movie/{id}/release_dates endpoint.
type ReleaseDatesResponse struct {
	ID      int                     `json:"id"`
	Results []ReleaseDatesByCountry `json:"results"`
}

// ReleaseDatesByCountry groups release dates by ISO 3166-1 country code.
type ReleaseDatesByCountry struct {
	ISO31661     string        `json:"iso_3166_1"`
	ReleaseDates []ReleaseDate `json:"release_dates"`
}

// ReleaseDate is a single release entry from TMDB.
// Type values: 1=Premiere, 2=Theatrical (limited), 3=Theatrical, 4=Digital, 5=Physical, 6=TV
type ReleaseDate struct {
	Type          int    `json:"type"`
	ReleaseDate   string `json:"release_date"` // ISO 8601 with time
	Certification string `json:"certification"`
}

// PersonResponse is the JSON response from TMDb /person/{id} endpoint.
type PersonResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Biography   string `json:"biography"`
	ProfilePath string `json:"profile_path"`
	Birthday    string `json:"birthday"`
	Deathday    string `json:"deathday"`
	KnownFor    string `json:"known_for_department"`
	PlaceOfBirth string `json:"place_of_birth"`
}

// CombinedCreditsResponse is the JSON response from TMDb /person/{id}/combined_credits.
type CombinedCreditsResponse struct {
	ID   int                    `json:"id"`
	Cast []CombinedCreditEntry  `json:"cast"`
	Crew []CombinedCreditEntry  `json:"crew"`
}

// CombinedCreditEntry is a single entry in a combined credits response.
type CombinedCreditEntry struct {
	ID           int     `json:"id"`
	MediaType    string  `json:"media_type"` // "movie" or "tv"
	Title        string  `json:"title,omitempty"`
	Name         string  `json:"name,omitempty"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	ReleaseDate  string  `json:"release_date,omitempty"`
	FirstAirDate string  `json:"first_air_date,omitempty"`
	VoteAverage  float64 `json:"vote_average"`
	Popularity   float64 `json:"popularity"`
	Character    string  `json:"character,omitempty"`
	Job          string  `json:"job,omitempty"`
	Department   string  `json:"department,omitempty"`
}
