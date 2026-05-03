package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Kind is the stable string under which a concrete download-client
// implementation registers itself (e.g. "qbittorrent", "sabnzbd",
// "builtin/null"). Stored in the download_clients.kind column.
type Kind string

// Built-in kinds that ship with the downloads core. Real kinds land
// in later phases and register themselves during their package init.
const (
	KindNull         Kind = "builtin/null"
	KindQBittorrent  Kind = "qbittorrent"
	KindTransmission Kind = "transmission"
	KindDeluge       Kind = "deluge"
	KindSABnzbd      Kind = "sabnzbd"
	KindNZBGet       Kind = "nzbget"
)

// Protocol is the wire family the kind speaks. We model only torrent
// vs usenet today; if more arrive (e.g. direct HTTP) they get their
// own values rather than overloading these.
type Protocol string

const (
	ProtocolTorrent Protocol = "torrent"
	ProtocolUsenet  Protocol = "usenet"
)

// HealthStatus mirrors the indexer status verbs so dashboards can
// render both subsystems uniformly. Stored in
// download_client_health.status.
type HealthStatus string

const (
	StatusOK       HealthStatus = "ok"
	StatusDegraded HealthStatus = "degraded"
	StatusFailed   HealthStatus = "failed"
	StatusUnknown  HealthStatus = "unknown"
)

// ItemStatus is the lifecycle phase a downloading item is in. The
// constants are deliberately small — kinds map their richer per-state
// vocabulary onto these.
type ItemStatus string

const (
	StatusItemQueued      ItemStatus = "queued"
	StatusItemDownloading ItemStatus = "downloading"
	StatusItemSeeding     ItemStatus = "seeding"
	StatusItemPaused      ItemStatus = "paused"
	StatusItemCompleted   ItemStatus = "completed"
	StatusItemFailed      ItemStatus = "failed"
	StatusItemUnknown     ItemStatus = "unknown"
)

// Category is a free-form grouping label (e.g. "movies", "tv-sonarr").
// The downloads package never inspects the value beyond round-tripping
// it; UIs and downstream automation drive the semantics.
type Category struct {
	Name     string `json:"name"`
	SavePath string `json:"save_path,omitempty"`
}

// AddRequest is the input to DownloadClient.Add. At least one of
// Magnet, TorrentURL, NZBURL, or RawBytes must be populated; the kind
// chooses which one to honour based on its Protocol.
type AddRequest struct {
	// Magnet is a torrent magnet URI. Torrent kinds prefer this
	// over TorrentURL when both are set.
	Magnet string `json:"magnet,omitempty"`

	// TorrentURL is a fetchable .torrent file URL. Torrent-only.
	TorrentURL string `json:"torrent_url,omitempty"`

	// NZBURL is a fetchable .nzb file URL. Usenet-only.
	NZBURL string `json:"nzb_url,omitempty"`

	// RawBytes is the literal payload (.torrent or .nzb body) when
	// the caller has the file in hand. Takes precedence over the
	// URL form when set.
	RawBytes []byte `json:"-"`

	// Title is a human-readable label persisted with the item.
	Title string `json:"title,omitempty"`

	// Category overrides Definition.CategoryDefault for this item.
	// Empty means "use the client default".
	Category string `json:"category,omitempty"`

	// SavePath overrides Definition.SavePathDefault. Empty means
	// "use the client default".
	SavePath string `json:"save_path,omitempty"`

	// Tags are passed through to kinds that support tagging.
	Tags []string `json:"tags,omitempty"`
}

// AddResult is the outcome of a successful DownloadClient.Add. The
// per-kind ID is opaque to the downloads package — callers store it
// and pass it back into Status/Pause/Resume/Remove.
type AddResult struct {
	ClientID string `json:"client_id"`
	ItemID   string `json:"item_id"`
}

// ItemStatus is a single in-flight or completed download as reported
// by a client. Field semantics map cleanly onto qBittorrent / SABnzbd
// status; kinds with less detail leave the unsupported fields zero.
type Item struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Category      string     `json:"category,omitempty"`
	Status        ItemStatus `json:"status"`
	Progress      float64    `json:"progress"` // 0.0 - 1.0
	SizeBytes     int64      `json:"size_bytes,omitempty"`
	DownloadedBytes int64    `json:"downloaded_bytes,omitempty"`
	ETA           int64      `json:"eta_seconds,omitempty"`
	DownloadRate  int64      `json:"download_rate,omitempty"`
	UploadRate    int64      `json:"upload_rate,omitempty"`
	Ratio         float64    `json:"ratio,omitempty"`
	Message       string     `json:"message,omitempty"`
}

// DownloadClient is the abstraction every download kind implements.
// Methods must be safe to call concurrently. Empty ids slice on
// Status/Pause/Resume means "all items".
type DownloadClient interface {
	ID() string
	Name() string
	Kind() Kind
	Protocol() Protocol

	Add(ctx context.Context, req AddRequest) (AddResult, error)
	Status(ctx context.Context, ids ...string) ([]Item, error)
	Pause(ctx context.Context, ids ...string) error
	Resume(ctx context.Context, ids ...string) error
	Remove(ctx context.Context, ids []string, deleteFiles bool) error

	Categories(ctx context.Context) ([]Category, error)

	// FreeSpace returns bytes available on the client's save path,
	// or -1 if the kind cannot report it.
	FreeSpace(ctx context.Context) (int64, error)

	// Test is a connectivity + auth check. nil = healthy.
	Test(ctx context.Context) error
}

// Definition is the persisted shape of a download_clients row. It is
// the hand-off between Repository and the rest of the package; sqlc
// produces engine-specific row types (different column kinds for
// booleans and JSON), so we project them onto this neutral struct.
type Definition struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Kind            Kind            `json:"kind"`
	Protocol        Protocol        `json:"protocol"`
	Enabled         bool            `json:"enabled"`
	Priority        int             `json:"priority"`
	Host            string          `json:"host,omitempty"`
	Port            int             `json:"port,omitempty"`
	TLS             bool            `json:"tls"`
	Username        string          `json:"username,omitempty"`
	Password        string          `json:"password,omitempty"`
	Config          json.RawMessage `json:"config,omitempty"`
	CategoryDefault string          `json:"category_default,omitempty"`
	SavePathDefault string          `json:"save_path_default,omitempty"`
	RemoveCompleted bool            `json:"remove_completed"`
	RemoveFailed    bool            `json:"remove_failed"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// Health is the persisted shape of a download_client_health row.
type Health struct {
	ClientID            string       `json:"client_id"`
	Status              HealthStatus `json:"status"`
	LastCheckedAt       time.Time    `json:"last_checked_at"`
	LastSuccessAt       *time.Time   `json:"last_success_at,omitempty"`
	LastFailureAt       *time.Time   `json:"last_failure_at,omitempty"`
	LastError           string       `json:"last_error,omitempty"`
	ConsecutiveFailures int          `json:"consecutive_failures"`
	LastFreeSpaceBytes  *int64       `json:"last_free_space_bytes,omitempty"`
	LastCategories      []Category   `json:"last_categories,omitempty"`
}

// Patch carries the optional fields acceptable on PATCH
// /api/v1/download-clients/{id}. Nil pointer = leave unchanged.
type Patch struct {
	ID              string
	Name            *string
	Enabled         *bool
	Priority        *int
	CategoryDefault *string
	SavePathDefault *string
	RemoveCompleted *bool
	RemoveFailed    *bool
}

// ErrNotFound is returned when a client ID has no row.
var ErrNotFound = errors.New("download client not found")

// ErrUnknownKind is returned when a Definition references a Kind that
// has not been registered.
var ErrUnknownKind = errors.New("unknown download client kind")
