package radarrv3

import "time"

// Radarr v3 API response/request models.

type radarrMovie struct {
	ID               int             `json:"id"`
	Title            string          `json:"title"`
	Year             int             `json:"year"`
	TmdbID           int             `json:"tmdbId"`
	ImdbID           string          `json:"imdbId,omitempty"`
	Overview         string          `json:"overview,omitempty"`
	Monitored        bool            `json:"monitored"`
	HasFile          bool            `json:"hasFile"`
	QualityProfileID int             `json:"qualityProfileId"`
	RootFolderPath   string          `json:"rootFolderPath,omitempty"`
	Path             string          `json:"path,omitempty"`
	Added            time.Time       `json:"added"`
	Status           string          `json:"status"`
	Images           []radarrImage   `json:"images"`
	Ratings          radarrRatings   `json:"ratings"`
	Runtime          int             `json:"runtime,omitempty"`
	Genres           []string        `json:"genres,omitempty"`
	SortTitle        string          `json:"sortTitle,omitempty"`
	SizeOnDisk       int64           `json:"sizeOnDisk"`
	IsAvailable      bool            `json:"isAvailable"`
	TitleSlug        string          `json:"titleSlug,omitempty"`
	FolderName       string          `json:"folderName,omitempty"`
	MovieFile        *radarrMoveFile `json:"movieFile,omitempty"`
}

type radarrMoveFile struct {
	ID           int    `json:"id"`
	MovieID      int    `json:"movieId"`
	RelativePath string `json:"relativePath,omitempty"`
	Path         string `json:"path,omitempty"`
	Size         int64  `json:"size"`
	Quality      any    `json:"quality,omitempty"`
	DateAdded    string `json:"dateAdded,omitempty"`
}

type radarrImage struct {
	CoverType string `json:"coverType"`
	URL       string `json:"url,omitempty"`
	RemoteURL string `json:"remoteUrl,omitempty"`
}

type radarrRatings struct {
	Votes int     `json:"votes"`
	Value float64 `json:"value"`
}

type radarrAddMovieRequest struct {
	Title            string `json:"title"`
	Year             int    `json:"year"`
	TmdbID           int    `json:"tmdbId"`
	ImdbID           string `json:"imdbId,omitempty"`
	QualityProfileID int    `json:"qualityProfileId"`
	RootFolderPath   string `json:"rootFolderPath"`
	Monitored        bool   `json:"monitored"`
	Overview         string `json:"overview,omitempty"`
}

type radarrRootFolder struct {
	ID              int                    `json:"id"`
	Path            string                 `json:"path"`
	Accessible      bool                   `json:"accessible"`
	FreeSpace       int64                  `json:"freeSpace"`
	UnmappedFolders []radarrUnmappedFolder `json:"unmappedFolders"`
}

type radarrUnmappedFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type radarrQualityProfile struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	UpgradeAllowed bool   `json:"upgradeAllowed"`
	Cutoff         int    `json:"cutoff"`
	Items          []any  `json:"items"`
}

type radarrCommand struct {
	ID       int       `json:"id"`
	Name     string    `json:"name"`
	Body     any       `json:"body,omitempty"`
	Status   string    `json:"status"`
	Queued   time.Time `json:"queued"`
	Started  time.Time `json:"started,omitempty"`
	Ended    time.Time `json:"ended,omitempty"`
	Priority string    `json:"priority"`
	Trigger  string    `json:"trigger"`
}

type radarrSystemStatus struct {
	AppName           string `json:"appName"`
	InstanceName      string `json:"instanceName"`
	Version           string `json:"version"`
	BuildTime         string `json:"buildTime"`
	IsDebug           bool   `json:"isDebug"`
	IsProduction      bool   `json:"isProduction"`
	IsAdmin           bool   `json:"isAdmin"`
	IsUserInteractive bool   `json:"isUserInteractive"`
	StartupPath       string `json:"startupPath"`
	AppData           string `json:"appData"`
	OsName            string `json:"osName"`
	OsVersion         string `json:"osVersion"`
	IsDocker          bool   `json:"isDocker"`
	IsLinux           bool   `json:"isLinux"`
	IsOsx             bool   `json:"isOsx"`
	IsWindows         bool   `json:"isWindows"`
	Authentication    string `json:"authentication"`
	URLBase           string `json:"urlBase"`
	RuntimeVersion    string `json:"runtimeVersion"`
	RuntimeName       string `json:"runtimeName"`
	Branch            string `json:"branch"`
	SqliteVersion     string `json:"sqliteVersion"`
	MigrationVersion  int    `json:"migrationVersion"`
}

type radarrTag struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}
