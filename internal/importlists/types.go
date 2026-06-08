package importlists

import "time"

// ListType enumerates the supported import list sources.
type ListType string

const (
	ListTypeTraktList        ListType = "trakt_list"
	ListTypeTraktWatchlist   ListType = "trakt_watchlist"
	ListTypeTraktPopular     ListType = "trakt_popular"
	ListTypeTraktTrending    ListType = "trakt_trending"
	ListTypeTraktAnticipated ListType = "trakt_anticipated"
	ListTypeIMDbList         ListType = "imdb_list"
	ListTypeIMDbWatchlist    ListType = "imdb_watchlist"
	ListTypeTMDbList         ListType = "tmdb_list"
	ListTypeTMDbPopular      ListType = "tmdb_popular"
	ListTypePlexWatchlist    ListType = "plex_watchlist"
	ListTypeRSS              ListType = "rss"
	ListTypeSonarr           ListType = "sonarr"
	ListTypeRadarr           ListType = "radarr"
)

// ListMode controls whether a list auto-adds items or only surfaces them in
// the Discover section for manual adding.
type ListMode string

const (
	// ListModeAuto auto-adds every fetched item to the library (legacy behaviour).
	ListModeAuto ListMode = "auto"
	// ListModeDiscover only lists fetched items; the user adds them manually.
	ListModeDiscover ListMode = "discover"
)

// MediaType is "movie" or "series".
type MediaType string

const (
	MediaTypeMovie  MediaType = "movie"
	MediaTypeSeries MediaType = "series"
	MediaTypeMusic  MediaType = "music"
)

// MonitorType controls what gets monitored after import.
type MonitorType string

const (
	MonitorAll     MonitorType = "all"
	MonitorFuture  MonitorType = "future"
	MonitorMissing MonitorType = "missing"
	MonitorNone    MonitorType = "none"
)

// ItemStatus tracks the state of an imported item.
type ItemStatus string

const (
	ItemStatusPending  ItemStatus = "pending"
	ItemStatusAdded    ItemStatus = "added"
	ItemStatusExcluded ItemStatus = "excluded"
	ItemStatusFailed   ItemStatus = "failed"
)

// ImportList represents a configured external list source.
type ImportList struct {
	ID                  string      `json:"id"`
	Name                string      `json:"name"`
	ListType            ListType    `json:"list_type"`
	Enabled             bool        `json:"enabled"`
	URL                 string      `json:"url,omitempty"`
	APIKey              string      `json:"api_key,omitempty"`
	AccessToken         string      `json:"access_token,omitempty"`
	SyncIntervalMinutes int         `json:"sync_interval_minutes"`
	LibraryPath         string      `json:"library_path,omitempty"`
	QualityProfileID    string      `json:"quality_profile_id"`
	MediaType           MediaType   `json:"media_type"`
	MonitorType         MonitorType `json:"monitor_type"`
	SearchOnAdd         bool        `json:"search_on_add"`
	Mode                ListMode    `json:"mode"`
	LastSync            *time.Time  `json:"last_sync,omitempty"`
	Settings            string      `json:"settings"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
}

// ImportListItem represents a single item fetched from an import list.
type ImportListItem struct {
	ID         string     `json:"id"`
	ListID     string     `json:"list_id"`
	ExternalID string     `json:"external_id"`
	Title      string     `json:"title"`
	Year       *int       `json:"year,omitempty"`
	IMDbID     string     `json:"imdb_id,omitempty"`
	TMDbID     string     `json:"tmdb_id,omitempty"`
	TVDbID     string     `json:"tvdb_id,omitempty"`
	MediaType  string     `json:"media_type,omitempty"`
	Status     ItemStatus `json:"status"`
	PosterPath string     `json:"poster_path,omitempty"`
	Overview   string     `json:"overview,omitempty"`
	Genres     []string   `json:"genres,omitempty"`
	LastSeen   time.Time  `json:"last_seen"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ImportListExclusion prevents an item from being re-added by any list.
type ImportListExclusion struct {
	ID        string    `json:"id"`
	TMDbID    string    `json:"tmdb_id,omitempty"`
	TVDbID    string    `json:"tvdb_id,omitempty"`
	IMDbID    string    `json:"imdb_id,omitempty"`
	Title     string    `json:"title"`
	Year      *int      `json:"year,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ListItem is the provider-agnostic item returned by providers.
type ListItem struct {
	ExternalID string `json:"external_id"`
	Title      string `json:"title"`
	Year       int    `json:"year,omitempty"`
	IMDbID     string `json:"imdb_id,omitempty"`
	TMDbID     string `json:"tmdb_id,omitempty"`
	TVDbID     string `json:"tvdb_id,omitempty"`
}
