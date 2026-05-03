package newznab

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Config is the parsed shape of the per-indexer config_json blob.
// Newznab and Torznab share the schema; only their default category
// hints differ.
//
// JSON tags mirror the documented schema in docs/indexers-newznab.md;
// users edit this through the API as opaque JSON.
type Config struct {
	URL         string           `json:"url"`
	APIKey      string           `json:"api_key"`
	UserAgent   string           `json:"user_agent,omitempty"`
	Timeout     durationString   `json:"timeout,omitempty"`
	CategoryMap map[string][]int `json:"category_map,omitempty"`
	// Internal: which attribute namespace this indexer publishes.
	// Populated by the kind factory, not by JSON.
	attrFlavour attrFlavour `json:"-"`
}

// durationString lets us accept "30s" / "2m" / "1h" in JSON while still
// surfacing a time.Duration to the rest of the package.
type durationString time.Duration

// UnmarshalJSON parses the string form ("30s") into a duration.
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
// the API doesn't surprise the user.
func (d durationString) MarshalJSON() ([]byte, error) {
	if d == 0 {
		return []byte(`""`), nil
	}
	return []byte(`"` + time.Duration(d).String() + `"`), nil
}

func (d durationString) duration() time.Duration { return time.Duration(d) }

// parseConfig validates raw, applies defaults, and tolerates two
// common operator slip-ups:
//
//  1. a trailing slash on URL (we strip it);
//  2. embedding `?apikey=...` in URL (we strip and merge into APIKey
//     when api_key was empty).
func parseConfig(raw json.RawMessage) (Config, error) {
	var cfg Config
	if len(raw) == 0 {
		return Config{}, errors.New("newznab: config is empty")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("newznab: decode config: %w", err)
	}
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		return Config{}, errors.New("newznab: url is required")
	}
	cleaned, embeddedKey, err := normaliseURL(cfg.URL)
	if err != nil {
		return Config{}, err
	}
	cfg.URL = cleaned
	if cfg.APIKey == "" && embeddedKey != "" {
		cfg.APIKey = embeddedKey
	}
	if cfg.APIKey == "" {
		return Config{}, errors.New("newznab: api_key is required")
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaultUserAgent
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = durationString(defaultTimeout)
	}
	return cfg, nil
}

// normaliseURL strips trailing slashes and any embedded apikey query
// parameter; it returns the cleaned URL plus the extracted key (if
// any).
func normaliseURL(in string) (cleaned, embeddedKey string, err error) {
	u, perr := url.Parse(in)
	if perr != nil {
		return "", "", fmt.Errorf("newznab: parse url %q: %w", in, perr)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("newznab: url %q must include scheme and host", in)
	}
	if v := u.Query(); v.Has("apikey") {
		embeddedKey = v.Get("apikey")
		v.Del("apikey")
		u.RawQuery = v.Encode()
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), embeddedKey, nil
}
