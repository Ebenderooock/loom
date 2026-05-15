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

// Family returns the top-level Newznab category family (e.g. 2040 → 2000).
func (c Category) Family() Category { return Category((int(c) / 1000) * 1000) }

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
//
// Torrent-specific fields (Infohash, Seeders, Peers, MagnetURI) are
// populated only by torrent-flavoured indexers (Torznab and friends).
// Usenet results leave them at their zero values: empty strings for
// Infohash/MagnetURI, nil for the *int counters. Use pointer-nil as
// the "not applicable" signal — a torrent with zero seeders is still
// distinguishable from a usenet release that has no seeder concept.
type Result struct {
	IndexerID string     `json:"indexer_id"`
	Title     string     `json:"title"`
	GUID      string     `json:"guid"`
	Link      string     `json:"link"`
	InfoURL   string     `json:"info_url,omitempty"`
	PubDate   time.Time  `json:"pub_date,omitempty"`
	Size      int64      `json:"size,omitempty"`
	Category  []Category `json:"categories,omitempty"`

	// Quality is a free-form release-quality / release-group string
	// (e.g. "1080p", "WEB-DL"). Optional; some indexers don't carry
	// it at all.
	Quality string `json:"quality,omitempty"`

	// Infohash is the BitTorrent v1 infohash as a 40-character
	// lowercase or uppercase hex string. Empty for Usenet results
	// and for torrent indexers that omit the attribute.
	Infohash string `json:"infohash,omitempty"`

	// Seeders is the upstream-reported seeder count for torrent
	// results. Nil for Usenet results, where the concept does not
	// apply. Zero (a non-nil pointer to 0) is a real torrent value
	// meaning "no seeders right now".
	Seeders *int `json:"seeders,omitempty"`

	// Peers is the upstream-reported peer count (seeders +
	// leechers, per the Torznab spec). Nil for Usenet results.
	// When upstream reports only `leechers` we synthesise this as
	// seeders + leechers so callers don't have to.
	Peers *int `json:"peers,omitempty"`

	// MagnetURI is an optional `magnet:` link supplied by some
	// trackers in addition to (or instead of) a `.torrent` Link.
	// Empty when the indexer does not advertise one.
	MagnetURI string `json:"magnet_uri,omitempty"`

	// Freeleech indicates the release does not count against the
	// user's download quota on the tracker (downloadvolumefactor=0).
	Freeleech bool `json:"freeleech,omitempty"`

	// Internal indicates the release was uploaded by the tracker's
	// internal encoding group / staff.
	Internal bool `json:"internal,omitempty"`

	// Scene indicates the release originates from the Scene
	// (pre-database match or explicit tracker tag).
	Scene bool `json:"scene,omitempty"`

	// Score is a computed ranking score (higher = better). Populated
	// by the scoring middleware, not by individual indexer
	// implementations.
	Score float64 `json:"score"`
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
	// ProxyID, when non-empty, references a row in the `proxies`
	// table and routes this indexer's outbound HTTP through that
	// proxy. Empty means "use the default transport" (Phase 2c
	// behaviour).
	ProxyID string `json:"proxy_id,omitempty"`
	// RequestDelay, when > 0, is the minimum milliseconds between
	// requests for this indexer (from the Cardigann YAML definition).
	// TransportForDefinition uses it to cap the throttle bucket rate.
	RequestDelay int       `json:"request_delay,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SeedConfig holds optional per-indexer seed policy overrides. These
// are stored inside the indexer's Config JSON blob and, when present,
// override the download client's default seed policy for grabs
// originating from this indexer.
type SeedConfig struct {
	RatioLimit       *float64 `json:"seed_ratio_limit,omitempty"`
	TimeLimitMinutes *int     `json:"seed_time_limit_minutes,omitempty"`
}

// ParseSeedConfig extracts seed policy overrides from an indexer
// definition's Config JSON. Returns a zero SeedConfig (both nil) if
// the definition has no config or no seed fields.
func ParseSeedConfig(def Definition) SeedConfig {
	if len(def.Config) == 0 {
		return SeedConfig{}
	}
	var sc SeedConfig
	_ = json.Unmarshal(def.Config, &sc)
	return sc
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

// Package-level sentinel errors that all indexer implementations
// (newznab, cardigann, etc.) should wrap so the service layer can
// classify failures uniformly without importing child packages.
var (
	// ErrIndexerTimeout indicates the request exceeded its deadline.
	// Implementations should wrap this (or context.DeadlineExceeded)
	// so the service marks the indexer as degraded, not failed.
	ErrIndexerTimeout = errors.New("indexer: timeout")

	// ErrIndexerRateLimited indicates an HTTP 429 or equivalent
	// throttle response. The service marks the indexer as degraded.
	ErrIndexerRateLimited = errors.New("indexer: rate limited")

	// ErrCloudFlareChallenge indicates the response is a Cloudflare
	// challenge page. The operator should configure a FlareSolverr
	// proxy for this indexer.
	ErrCloudFlareChallenge = errors.New("indexer: cloudflare challenge detected")
)

// IsTimeoutErr returns true if err represents a timeout — either the
// package-level sentinel, context.DeadlineExceeded, or a net.Error
// with Timeout() == true.
func IsTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrIndexerTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// net.Error covers http.Client.Timeout and dial timeouts.
	type netErr interface{ Timeout() bool }
	var ne netErr
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

// IsRateLimitErr returns true if err represents rate limiting.
func IsRateLimitErr(err error) bool {
	return err != nil && errors.Is(err, ErrIndexerRateLimited)
}

// CardigannDefSummary is a lightweight projection of a Cardigann YAML
// definition used by the "list available definitions" API endpoint.
type CardigannDefSummary struct {
	ID          string
	Name        string
	Description string
	Type        string
	Language    string
	Links       []string
	Settings    []CardigannSettingSummary
	Categories  []string // top-level Newznab categories (e.g. "Movies", "TV", "Audio")
}

// CardigannSettingSummary is one credential/option field from a YAML
// definition.
type CardigannSettingSummary struct {
	Name    string
	Type    string
	Label   string
	Default string
}

// DefinitionLister abstracts the ability to list available Cardigann
// definitions. The cardigann package implements this so the handlers
// package can use it without importing cardigann (avoiding a cycle).
type DefinitionLister interface {
	ListDefinitions() []CardigannDefSummary
}
