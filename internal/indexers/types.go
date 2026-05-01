package indexers

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Kind is the stable string under which a concrete indexer
// implementation registers itself (e.g. "builtin/null", "newznab",
// "cardigann"). Stored in the indexers.kind column.
type Kind string

// Built-in kinds that ship with the indexer core. Real kinds
// (Cardigann, Newznab, Torznab) land in later phases and register
// themselves during their package init.
const (
	KindNull Kind = "builtin/null"
)

// HealthStatus enumerates the verbs surfaced via the API and stored in
// indexer_health.status. Order matters for promotion logic in the
// HealthChecker (ok > degraded > failed > unknown).
type HealthStatus string

const (
	StatusOK       HealthStatus = "ok"
	StatusDegraded HealthStatus = "degraded"
	StatusFailed   HealthStatus = "failed"
	StatusUnknown  HealthStatus = "unknown"
)

// Category is the Newznab-aligned numeric taxonomy. The constants
// below mirror the canonical Newznab category IDs so external clients
// (Radarr / Sonarr / Lidarr) get the numbers they expect.
//
// See https://github.com/Sonarr/Sonarr/wiki/Indexers for the upstream
// list. We model only the families Loom needs today; sub-categories
// (e.g. 2040 = Movies/HD) are passed through as raw integers and not
// pre-declared.
type Category int

const (
	CategoryConsole Category = 1000
	CategoryMovies  Category = 2000
	CategoryAudio   Category = 3000
	CategoryPC      Category = 4000
	CategoryTV      Category = 5000
	CategoryXXX     Category = 6000
	CategoryBooks   Category = 7000
	CategoryOther   Category = 8000
)

// CategoryFamilies returns the well-known Newznab category families in
// numeric order. Useful for UI dropdowns and OpenAPI enums.
func CategoryFamilies() []Category {
	return []Category{
		CategoryConsole,
		CategoryMovies,
		CategoryAudio,
		CategoryPC,
		CategoryTV,
		CategoryXXX,
		CategoryBooks,
		CategoryOther,
	}
}

// Caps describes what a concrete indexer can do. Reported by Caps()
// and surfaced via GET /api/v1/indexers/{id}/caps.
type Caps struct {
	// SearchTypes is the set of Newznab-style search modes the
	// indexer answers (e.g. "search", "tv-search", "movie-search").
	SearchTypes []string `json:"search_types"`

	// Categories is the list of category IDs the indexer accepts.
	Categories []Category `json:"categories"`

	// SupportedIDs lists the external ID schemes the indexer
	// understands (e.g. "imdb", "tvdb", "tmdb").
	SupportedIDs []string `json:"supported_ids"`
}

// Query is the input to Search. Zero-valued fields mean "no filter".
type Query struct {
	// Term is the free-text search string.
	Term string `json:"query"`

	// Categories restricts results to the given Newznab categories;
	// empty means any.
	Categories []Category `json:"categories,omitempty"`

	// IMDBID, TVDBID, TMDBID are external IDs an indexer may use to
	// disambiguate a search. Empty values are ignored.
	IMDBID string `json:"imdb_id,omitempty"`
	TVDBID string `json:"tvdb_id,omitempty"`
	TMDBID string `json:"tmdb_id,omitempty"`

	// Season and Episode allow per-episode lookups for TV. Zero
	// means unset.
	Season  int `json:"season,omitempty"`
	Episode int `json:"episode,omitempty"`

	// Limit caps the rows returned by a single indexer. Zero means
	// "indexer default".
	Limit int `json:"limit,omitempty"`
}

// Result is one row from a single indexer's search response. Field
// names follow the Newznab item shape closely so wire-compat layers
// can map cheaply.
type Result struct {
	IndexerID string     `json:"indexer_id"`
	Title     string     `json:"title"`
	GUID      string     `json:"guid"`
	Link      string     `json:"link"`
	InfoURL   string     `json:"info_url,omitempty"`
	PubDate   time.Time  `json:"pub_date,omitempty"`
	Size      int64      `json:"size,omitempty"`
	Seeders   int        `json:"seeders,omitempty"`
	Peers     int        `json:"peers,omitempty"`
	Category  []Category `json:"categories,omitempty"`
	Quality   string     `json:"quality,omitempty"`
}

// Results is the whole-of-search response from a single indexer. Total
// is the upstream-reported total (may exceed len(Items) if the indexer
// paginated and we truncated).
type Results struct {
	IndexerID string   `json:"indexer_id"`
	Items     []Result `json:"items"`
	Total     int      `json:"total"`
}

// Indexer is the abstraction every search source implements. The
// methods must be safe to call concurrently.
type Indexer interface {
	// ID returns the stable identifier (slug or UUID) the indexer is
	// stored under.
	ID() string

	// Name returns the human-readable name shown in the UI.
	Name() string

	// Caps returns the indexer's declared capabilities. Callers may
	// cache the value across Search calls.
	Caps() Caps

	// Search runs q against the indexer. Implementations must honour
	// ctx cancellation; partial results before cancellation are
	// allowed but errors should still be returned.
	Search(ctx context.Context, q Query) (*Results, error)

	// Test performs a connectivity + auth check. A nil return means
	// the indexer is reachable and authenticated.
	Test(ctx context.Context) error
}

// Definition is the persisted shape of an indexer row. It is the
// hand-off between Repository and the rest of the package — sqlc
// produces engine-specific types (different column kinds for booleans
// and JSON), so we project them onto this neutral struct.
type Definition struct {
	ID         string          `json:"id"`
	Kind       Kind            `json:"kind"`
	Name       string          `json:"name"`
	Enabled    bool            `json:"enabled"`
	Priority   int             `json:"priority"`
	Config     json.RawMessage `json:"config"`
	Categories []Category      `json:"categories"`
	Tags       []string        `json:"tags"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// Health is the persisted shape of an indexer_health row.
type Health struct {
	IndexerID     string        `json:"indexer_id"`
	Status        HealthStatus  `json:"status"`
	LastCheckedAt time.Time     `json:"last_checked_at"`
	LastSuccessAt *time.Time    `json:"last_success_at,omitempty"`
	Latency       time.Duration `json:"-"`
	LatencyMS     int64         `json:"latency_ms,omitempty"`
	LastError     string        `json:"last_error,omitempty"`
}

// ErrNotFound is returned when an indexer ID has no row.
var ErrNotFound = errors.New("indexer not found")

// ErrUnknownKind is returned when a Definition references a Kind that
// has not been registered.
var ErrUnknownKind = errors.New("unknown indexer kind")
