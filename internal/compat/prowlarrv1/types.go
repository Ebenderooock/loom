package prowlarrv1

import "time"

// --- Prowlarr v1 wire types ---

// prowlarrIndexer is the Prowlarr v1 JSON shape for an indexer.
type prowlarrIndexer struct {
	ID                 int             `json:"id"`
	Name               string          `json:"name"`
	Protocol           string          `json:"protocol"`
	Enable             bool            `json:"enable"`
	Priority           int             `json:"priority"`
	AppProfileID       int             `json:"appProfileId"`
	Fields             []prowlarrField `json:"fields"`
	ImplementationName string          `json:"implementationName"`
	Implementation     string          `json:"implementation"`
	ConfigContract     string          `json:"configContract"`
	Tags               []int           `json:"tags"`
}

type prowlarrField struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// prowlarrSearchResult is the Prowlarr v1 JSON shape for a search
// result item.
type prowlarrSearchResult struct {
	GUID        string             `json:"guid"`
	IndexerID   int                `json:"indexerId"`
	Title       string             `json:"title"`
	SortTitle   string             `json:"sortTitle"`
	Size        int64              `json:"size"`
	PublishDate string             `json:"publishDate"`
	DownloadURL string             `json:"downloadUrl"`
	InfoURL     string             `json:"infoUrl,omitempty"`
	Categories  []prowlarrCategory `json:"categories"`
	Protocol    string             `json:"protocol"`
	Seeders     *int               `json:"seeders,omitempty"`
	Leechers    *int               `json:"leechers,omitempty"`
	MagnetURL   string             `json:"magnetUrl,omitempty"`
}

type prowlarrCategory struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// prowlarrSystemStatus is the Prowlarr v1 system/status response.
type prowlarrSystemStatus struct {
	AppName           string `json:"appName"`
	Version           string `json:"version"`
	BuildTime         string `json:"buildTime"`
	IsDebug           bool   `json:"isDebug"`
	IsProduction      bool   `json:"isProduction"`
	IsAdmin           bool   `json:"isAdmin"`
	IsUserInteractive bool   `json:"isUserInteractive"`
	StartupPath       string `json:"startupPath"`
	AppData           string `json:"appData"`
	OsName            string `json:"osName"`
	IsDocker          bool   `json:"isDocker"`
	Branch            string `json:"branch"`
	Authentication    string `json:"authentication"`
	URLBase           string `json:"urlBase"`
	RuntimeVersion    string `json:"runtimeVersion"`
	RuntimeName       string `json:"runtimeName"`
	StartTime         string `json:"startTime"`
}

// prowlarrHealth is the Prowlarr v1 health check response item.
type prowlarrHealth struct {
	Source  string `json:"source"`
	Type    string `json:"type"`
	Message string `json:"message"`
	WikiURL string `json:"wikiUrl"`
}

// prowlarrIndexerStats is the Prowlarr v1 indexer stats response.
type prowlarrIndexerStats struct {
	Indexers []prowlarrIndexerStat `json:"indexers"`
}

type prowlarrIndexerStat struct {
	IndexerID           int    `json:"indexerId"`
	IndexerName         string `json:"indexerName"`
	AverageResponseTime int    `json:"averageResponseTime"`
	NumberOfQueries     int    `json:"numberOfQueries"`
	NumberOfGrabs       int    `json:"numberOfGrabs"`
	NumberOfFailures    int    `json:"numberOfFailures"`
}

var startTime = time.Now()

// prowlarrApplication is the Prowlarr v1 JSON shape for an application
// (downstream app like Radarr or Sonarr).
type prowlarrApplication struct {
	ID             int             `json:"id"`
	Name           string          `json:"name"`
	SyncLevel      string          `json:"syncLevel,omitempty"`
	BaseURL        string          `json:"baseUrl,omitempty"`
	ProwlarrURL    string          `json:"prowlarrUrl,omitempty"`
	APIKey         string          `json:"apiKey,omitempty"`
	AppProfileID   int             `json:"appProfileId,omitempty"`
	Tags           []int           `json:"tags"`
	Fields         []prowlarrField `json:"fields,omitempty"`
	Implementation string          `json:"implementation,omitempty"`
	ConfigContract string          `json:"configContract,omitempty"`
}
