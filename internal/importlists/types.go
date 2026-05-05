package importlists

import "time"

// ListType enumerates the supported import list sources.
type ListType string

const (
	ListTypeTraktList      ListType = "trakt_list"
	ListTypeTraktWatchlist ListType = "trakt_watchlist"
	ListTypeIMDbList       ListType = "imdb_list"
	ListTypeIMDbWatchlist  ListType = "imdb_watchlist"
	ListTypeTMDbList       ListType = "tmdb_list"
	ListTypeTMDbPopular    ListType = "tmdb_popular"
	ListTypePlexWatchlist  ListType = "plex_watchlist"
	ListTypeRSS            ListType = "rss"
	ListTypeSonarr         ListType = "sonarr"
	ListTypeRadarr         ListType = "radarr"
)

// MediaType is "movie" or "series".
type MediaType string

const (
	MediaTypeMovie  MediaType = "movie"
	MediaTypeSeries MediaType = "series"
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
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	ListType            ListType   `json:"list_type"`
	Enabled             bool       `json:"enabled"`
	URL                 string     `json:"url,omitempty"`
	APIKey              string     `json:"api_key,omitempty"`
	AccessToken         string     `json:"access_token,omitempty"`
	SyncIntervalMinutes int        `json:"sync_interval_minutes"`
	RootFolderPath      string     `json:"root_folder_path,omitempty"`
	QualityProfileID    string     `json:"quality_profile_id"`
	MediaType           MediaType  `json:"media_type"`
	MonitorType         MonitorType `json:"monitor_type"`
	SearchOnAdd         bool       `json:"search_on_add"`
	LastSync            *time.Time `json:"last_sync,omitempty"`
	Settings            string     `json:"settings"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
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
	Status     ItemStatus `json:"status"`
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
