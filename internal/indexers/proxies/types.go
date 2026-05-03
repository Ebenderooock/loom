package proxies

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Kind names the supported transport implementations. Stored as a
// plain string in the `proxies.kind` column.
type Kind string

const (
	KindHTTP         Kind = "http"
	KindHTTPS        Kind = "https"
	KindSOCKS5       Kind = "socks5"
	KindFlareSolverr Kind = "flaresolverr"
)

// Proxy is the persisted record. It is engine-neutral — sqlc
// emits two struct shapes (one per engine) that the repository
// projects onto this type.
type Proxy struct {
	ID        string          `json:"id"`
	Kind      Kind            `json:"kind"`
	Name      string          `json:"name"`
	Enabled   bool            `json:"enabled"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// HTTPConfig backs proxy.kind in {"http","https"}. URL must include
// the scheme; Username/Password are optional and applied as
// Proxy-Authorization basic credentials by the standard library.
type HTTPConfig struct {
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// SOCKS5Config backs proxy.kind == "socks5".
type SOCKS5Config struct {
	Address  string `json:"address"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// FlareSolverrConfig backs proxy.kind == "flaresolverr".
//
// MaxTimeoutSec, when zero, falls back to the package default (set
// from kernel/config indexers.proxies.flaresolverr_default_timeout).
// SessionMode controls whether the package opens / reuses / drops a
// FlareSolverr session per request: "" (== "none") issues a stateless
// `request.get`; "shared" creates one session per Proxy row and
// reuses it across requests.
type FlareSolverrConfig struct {
	URL           string `json:"url"`
	MaxTimeoutSec int    `json:"max_timeout_sec,omitempty"`
	SessionMode   string `json:"session_mode,omitempty"`
}

// ParseHTTPConfig validates and decodes an http/https config blob.
func ParseHTTPConfig(raw json.RawMessage) (HTTPConfig, error) {
	var c HTTPConfig
	if err := decodeStrict(raw, &c); err != nil {
		return HTTPConfig{}, err
	}
	if strings.TrimSpace(c.URL) == "" {
		return HTTPConfig{}, errors.New("http proxy: url is required")
	}
	u, err := url.Parse(c.URL)
	if err != nil {
		return HTTPConfig{}, fmt.Errorf("http proxy: invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return HTTPConfig{}, fmt.Errorf("http proxy: url scheme must be http or https, got %q", u.Scheme)
	}
	return c, nil
}

// ParseSOCKS5Config validates and decodes a socks5 config blob.
func ParseSOCKS5Config(raw json.RawMessage) (SOCKS5Config, error) {
	var c SOCKS5Config
	if err := decodeStrict(raw, &c); err != nil {
		return SOCKS5Config{}, err
	}
	if strings.TrimSpace(c.Address) == "" {
		return SOCKS5Config{}, errors.New("socks5 proxy: address is required")
	}
	if !strings.Contains(c.Address, ":") {
		return SOCKS5Config{}, fmt.Errorf("socks5 proxy: address %q is missing :port", c.Address)
	}
	return c, nil
}

// ParseFlareSolverrConfig validates and decodes a flaresolverr blob.
func ParseFlareSolverrConfig(raw json.RawMessage) (FlareSolverrConfig, error) {
	var c FlareSolverrConfig
	if err := decodeStrict(raw, &c); err != nil {
		return FlareSolverrConfig{}, err
	}
	if strings.TrimSpace(c.URL) == "" {
		return FlareSolverrConfig{}, errors.New("flaresolverr proxy: url is required")
	}
	if _, err := url.Parse(c.URL); err != nil {
		return FlareSolverrConfig{}, fmt.Errorf("flaresolverr proxy: invalid url: %w", err)
	}
	switch c.SessionMode {
	case "", "none", "shared":
	default:
		return FlareSolverrConfig{}, fmt.Errorf("flaresolverr proxy: unknown session_mode %q", c.SessionMode)
	}
	return c, nil
}

// ValidateConfig dispatches to the kind-specific parser and returns
// the canonical (re-marshalled) JSON used for storage.
func ValidateConfig(kind Kind, raw json.RawMessage) (json.RawMessage, error) {
	switch kind {
	case KindHTTP, KindHTTPS:
		c, err := ParseHTTPConfig(raw)
		if err != nil {
			return nil, err
		}
		return json.Marshal(c)
	case KindSOCKS5:
		c, err := ParseSOCKS5Config(raw)
		if err != nil {
			return nil, err
		}
		return json.Marshal(c)
	case KindFlareSolverr:
		c, err := ParseFlareSolverrConfig(raw)
		if err != nil {
			return nil, err
		}
		return json.Marshal(c)
	default:
		return nil, fmt.Errorf("unknown proxy kind %q", kind)
	}
}

func decodeStrict(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	return nil
}

// Patch carries the optional fields acceptable on PATCH /proxies/{id}.
type Patch struct {
	ID      string
	Kind    *Kind
	Name    *string
	Enabled *bool
	Config  *json.RawMessage
}

// ErrNotFound is returned when a proxy ID has no row.
var ErrNotFound = errors.New("proxy not found")

// ErrInUse is returned by Delete when one or more indexers still
// reference the proxy. The wrapped IndexerIDs slice is exposed via
// errors.As.
type ErrInUse struct {
	IndexerIDs []string
}

func (e *ErrInUse) Error() string {
	return fmt.Sprintf("proxy in use by %d indexer(s)", len(e.IndexerIDs))
}
