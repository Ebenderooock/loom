package discover

import "time"

type Recommendation struct {
	TMDBID       int      `json:"tmdb_id"`
	MediaType    string   `json:"media_type"`
	Title        string   `json:"title"`
	Year         int      `json:"year"`
	PosterPath   string   `json:"poster_path,omitempty"`
	Overview     string   `json:"overview,omitempty"`
	Rating       float64  `json:"rating"`
	Popularity   float64  `json:"popularity"`
	ReleaseDate  string   `json:"release_date,omitempty"`
	Reasons      []string `json:"reasons"`
	Source       string   `json:"source"`
	CollectionID int      `json:"collection_id,omitempty"`
}

type ResultStatus struct {
	Warnings    []string  `json:"warnings"`
	GeneratedAt time.Time `json:"generated_at"`
	Stale       bool      `json:"stale"`
}

type RecommendationsResponse struct {
	Items []Recommendation `json:"items"`
	ResultStatus
}

type Collection struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Overview   string `json:"overview,omitempty"`
	PosterPath string `json:"poster_path,omitempty"`
	Total      int    `json:"total"`
	InLibrary  int    `json:"in_library"`
	Missing    int    `json:"missing"`
	Upcoming   int    `json:"upcoming"`
}

type CollectionItem struct {
	Recommendation
	InLibrary bool   `json:"in_library"`
	MediaID   string `json:"media_id,omitempty"`
	HasFile   *bool  `json:"has_file,omitempty"`
	Monitored bool   `json:"monitored"`
}

type CollectionsResponse struct {
	Items []Collection `json:"items"`
	ResultStatus
}

type CollectionResponse struct {
	Collection Collection       `json:"collection"`
	Items      []CollectionItem `json:"items"`
	ResultStatus
}
