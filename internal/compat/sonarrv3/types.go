package sonarrv3

import "time"

// --- Sonarr v3 wire types ---

// sonarrSeries is the Sonarr v3 JSON shape for a series.
type sonarrSeries struct {
	ID                int              `json:"id"`
	Title             string           `json:"title"`
	SortTitle         string           `json:"sortTitle"`
	Year              int              `json:"year"`
	TvdbID            int              `json:"tvdbId"`
	ImdbID            string           `json:"imdbId"`
	TvMazeID          int              `json:"tvMazeId"`
	Overview          string           `json:"overview"`
	Network           string           `json:"network,omitempty"`
	Runtime           int              `json:"runtime"`
	Monitored         bool             `json:"monitored"`
	QualityProfileID  int              `json:"qualityProfileId"`
	LanguageProfileID int              `json:"languageProfileId"`
	RootFolderPath    string           `json:"rootFolderPath"`
	Path              string           `json:"path"`
	Added             string           `json:"added"`
	Status            string           `json:"status"`
	SeriesType        string           `json:"seriesType"`
	SeasonFolder      bool             `json:"seasonFolder"`
	Genres            []string         `json:"genres"`
	Tags              []int            `json:"tags"`
	Seasons           []sonarrSeason   `json:"seasons"`
	Images            []sonarrImage    `json:"images"`
	Ratings           sonarrRatings    `json:"ratings"`
	Statistics        sonarrStatistics `json:"statistics"`
}

// sonarrSeason is a season entry inside a Sonarr series response.
type sonarrSeason struct {
	SeasonNumber int  `json:"seasonNumber"`
	Monitored    bool `json:"monitored"`
}

// sonarrImage is an image entry inside a Sonarr series response.
type sonarrImage struct {
	CoverType string `json:"coverType"`
	URL       string `json:"url"`
}

// sonarrRatings represents ratings in a Sonarr series response.
type sonarrRatings struct {
	Votes int     `json:"votes"`
	Value float64 `json:"value"`
}

// sonarrStatistics holds episode statistics in a Sonarr series response.
type sonarrStatistics struct {
	SeasonCount       int     `json:"seasonCount"`
	EpisodeCount      int     `json:"episodeCount"`
	EpisodeFileCount  int     `json:"episodeFileCount"`
	PercentOfEpisodes float64 `json:"percentOfEpisodes"`
	TotalEpisodeCount int     `json:"totalEpisodeCount"`
}

// sonarrEpisode is the Sonarr v3 JSON shape for an episode.
type sonarrEpisode struct {
	ID            int    `json:"id"`
	SeriesID      int    `json:"seriesId"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	Title         string `json:"title"`
	Overview      string `json:"overview"`
	AirDate       string `json:"airDate"`
	Monitored     bool   `json:"monitored"`
	HasFile       bool   `json:"hasFile"`
}

// sonarrRootFolder is the Sonarr v3 JSON shape for a root folder.
type sonarrRootFolder struct {
	ID         int    `json:"id"`
	Path       string `json:"path"`
	Accessible bool   `json:"accessible"`
	FreeSpace  int64  `json:"freeSpace"`
}

// sonarrQualityProfile is the Sonarr v3 JSON shape for a quality profile.
type sonarrQualityProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// sonarrCommand is the Sonarr v3 JSON shape for a command response.
type sonarrCommand struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	StartedOn       string `json:"startedOn"`
	StateChangeTime string `json:"stateChangeTime"`
}

// sonarrSystemStatus is the Sonarr v3 system/status response.
type sonarrSystemStatus struct {
	AppName        string `json:"appName"`
	Version        string `json:"version"`
	BuildTime      string `json:"buildTime"`
	IsDebug        bool   `json:"isDebug"`
	IsProduction   bool   `json:"isProduction"`
	IsAdmin        bool   `json:"isAdmin"`
	IsDocker       bool   `json:"isDocker"`
	Branch         string `json:"branch"`
	Authentication string `json:"authentication"`
	URLBase        string `json:"urlBase"`
	RuntimeVersion string `json:"runtimeVersion"`
	RuntimeName    string `json:"runtimeName"`
	StartTime      string `json:"startTime"`
	OsName         string `json:"osName"`
}

// sonarrLanguageProfile is the Sonarr v3 JSON shape for a language profile.
type sonarrLanguageProfile struct {
	ID             int              `json:"id"`
	Name           string           `json:"name"`
	UpgradeAllowed bool             `json:"upgradeAllowed"`
	Cutoff         sonarrLanguage   `json:"cutoff"`
	Languages      []sonarrLangItem `json:"languages"`
}

// sonarrLangItem is a single language entry in a language profile.
type sonarrLangItem struct {
	Language sonarrLanguage `json:"language"`
	Allowed  bool           `json:"allowed"`
}

// sonarrLanguage identifies a language by ID and name.
type sonarrLanguage struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// sonarrAddSeriesRequest is the expected JSON body for POST /api/v3/series.
type sonarrAddSeriesRequest struct {
	Title             string         `json:"title"`
	TvdbID            int            `json:"tvdbId"`
	QualityProfileID  int            `json:"qualityProfileId"`
	LanguageProfileID int            `json:"languageProfileId"`
	RootFolderPath    string         `json:"rootFolderPath"`
	Monitored         bool           `json:"monitored"`
	SeriesType        string         `json:"seriesType"`
	SeasonFolder      *bool          `json:"seasonFolder,omitempty"`
	Seasons           []sonarrSeason `json:"seasons,omitempty"`
	AddOptions        *struct {
		SearchForMissingEpisodes bool `json:"searchForMissingEpisodes"`
	} `json:"addOptions,omitempty"`
}

var startTime = time.Now()
