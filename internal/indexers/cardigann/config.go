package cardigann

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/loomctl/loom/internal/indexers"
)

// Config is the per-indexer JSON blob persisted in the indexers table.
//
// `definition_id` (or its alias `definition`) names the YAML file under
// the data dir's definitions folder, sans extension. Credentials are
// kept in a free-form map so trackers that need passkeys, RSS keys, or
// 2FA secrets do not require schema changes here.
//
// CategoryOverrides lets an operator pin a single tracker category to
// a different Newznab id without editing the YAML — useful when the
// upstream definition mismaps a category for the local content.
type Config struct {
	DefinitionID      string            `json:"definition_id,omitempty"`
	Definition        string            `json:"definition,omitempty"` // alias accepted for friendliness
	URL               string            `json:"url,omitempty"`        // overrides definition Links[0]
	UserAgent         string            `json:"user_agent,omitempty"`
	Timeout           durationString    `json:"timeout,omitempty"`
	Username          string            `json:"username,omitempty"`
	Password          string            `json:"password,omitempty"`
	Passkey           string            `json:"passkey,omitempty"`
	Cookie            string            `json:"cookie,omitempty"`
	Credentials       map[string]string `json:"credentials,omitempty"`
	CategoryOverrides map[string]int    `json:"category_overrides,omitempty"`
}

// durationString matches the newznab package's helper so the JSON
// duration format is consistent across kinds.
type durationString time.Duration

// UnmarshalJSON parses "30s" / "2m" / "1h" into a Duration.
func (d *durationString) UnmarshalJSON(raw []byte) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	s := strings.Trim(string(raw), `"`)
	if s == "" {
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = durationString(v)
	return nil
}

// MarshalJSON emits the canonical "30s" form so a round-trip through
// the API returns the exact value the operator wrote.
func (d durationString) MarshalJSON() ([]byte, error) {
	if d == 0 {
		return []byte(`""`), nil
	}
	return []byte(`"` + time.Duration(d).String() + `"`), nil
}

func (d durationString) duration() time.Duration { return time.Duration(d) }

// fields packages the operator-supplied credential map for use in
// login/search input templates ({{ .Config.username }}, etc.). We
// fold the named convenience fields and the free-form map into one
// flat dictionary; explicit credentials win on conflict.
func (c Config) fields() map[string]string {
	out := map[string]string{}
	for k, v := range c.Credentials {
		out[k] = v
	}
	if c.Username != "" {
		out["username"] = c.Username
	}
	if c.Password != "" {
		out["password"] = c.Password
	}
	if c.Passkey != "" {
		out["passkey"] = c.Passkey
	}
	if c.Cookie != "" {
		out["cookie"] = c.Cookie
	}
	return out
}

// resolvedDefinitionID prefers the DefinitionID field, falling back
// to the legacy `definition` alias.
func (c Config) resolvedDefinitionID() string {
	if id := strings.TrimSpace(c.DefinitionID); id != "" {
		return id
	}
	return strings.TrimSpace(c.Definition)
}

// parseConfig validates raw, applies defaults.
func parseConfig(raw json.RawMessage) (Config, error) {
	var cfg Config
	if len(raw) == 0 {
		return Config{}, errors.New("cardigann: config is empty")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("cardigann: decode config: %w", err)
	}
	if cfg.resolvedDefinitionID() == "" {
		return Config{}, errors.New("cardigann: definition_id is required")
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaultUserAgent
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = durationString(defaultTimeout)
	}
	return cfg, nil
}

// knownCategoryNames maps the Newznab category-name strings used in
// Cardigann's `categorymappings:` block to their numeric IDs. We
// cover the standard families and the most-used HD/SD subdivisions;
// definitions targeting a sub-category we do not list resolve to the
// parent family, which is good enough for fan-out filtering.
var knownCategoryNames = map[string]indexers.Category{
	// Console family
	"Console":            indexers.CategoryConsole,
	"Console/NDS":        1010,
	"Console/PSP":        1020,
	"Console/Wii":        1030,
	"Console/XBox":       1040,
	"Console/XBox 360":   1050,
	"Console/Wiiware/V":  1060,
	"Console/XBox 360 DLC": 1070,
	"Console/PS3":        1080,
	"Console/Other":      1999,

	// Movies family
	"Movies":      indexers.CategoryMovies,
	"Movies/Foreign": 2010,
	"Movies/Other":   2020,
	"Movies/SD":      2030,
	"Movies/HD":      2040,
	"Movies/3D":      2045,
	"Movies/UHD":     2045, // some defs use this name for 4K
	"Movies/BluRay":  2050,
	"Movies/DVD":     2060,
	"Movies/WEB-DL":  2070,

	// Audio family
	"Audio":         indexers.CategoryAudio,
	"Audio/MP3":     3010,
	"Audio/Video":   3020,
	"Audio/Audiobook": 3030,
	"Audio/Lossless": 3040,
	"Audio/Other":    3999,

	// PC family
	"PC":         indexers.CategoryPC,
	"PC/0day":    4010,
	"PC/ISO":     4020,
	"PC/Mac":     4030,
	"PC/Phone-Other": 4040,
	"PC/Games":   4050,
	"PC/Phone-IOS": 4060,
	"PC/Phone-Android": 4070,

	// TV family
	"TV":         indexers.CategoryTV,
	"TV/WEB-DL":  5010,
	"TV/FOREIGN": 5020,
	"TV/SD":      5030,
	"TV/HD":      5040,
	"TV/UHD":     5045,
	"TV/Other":   5050,
	"TV/Sport":   5060,
	"TV/Anime":   5070,
	"TV/Documentary": 5080,

	// XXX family
	"XXX":     indexers.CategoryXXX,
	"XXX/DVD": 6010,
	"XXX/WMV": 6020,
	"XXX/XviD": 6030,
	"XXX/x264": 6040,
	"XXX/UHD":  6045,
	"XXX/Pack": 6050,
	"XXX/ImageSet": 6060,
	"XXX/Other": 6070,
	"XXX/SD":    6080,
	"XXX/WEB-DL": 6090,

	// Books family
	"Books":      indexers.CategoryBooks,
	"Books/Mags": 7010,
	"Books/EBook": 7020,
	"Books/Comics": 7030,
	"Books/Technical": 7040,
	"Books/Other": 7050,
	"Books/Foreign": 7060,

	// Other family
	"Other":      indexers.CategoryOther,
	"Other/Misc": 8010,
	"Other/Hashed": 8020,
}

// newznabCategoryFromName looks up a Cardigann category-name string
// (e.g. "Movies/HD") in the known table. Lookup is case-insensitive
// because real-world definitions sometimes write "Movies/Sd".
func newznabCategoryFromName(name string) (indexers.Category, bool) {
	name = strings.TrimSpace(name)
	if c, ok := knownCategoryNames[name]; ok {
		return c, true
	}
	lower := strings.ToLower(name)
	for k, v := range knownCategoryNames {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return 0, false
}
