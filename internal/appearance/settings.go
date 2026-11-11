package appearance

import (
	"fmt"
	"net/url"
	"regexp"
)

// BackgroundPath is the internal, authenticated path that serves the uploaded
// background image. Settings may reference it, optionally with ?v=<sha256>.
const BackgroundPath = "/api/v1/appearance/background"

var (
	hexColorPattern        = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	internalBackgroundExpr = regexp.MustCompile(`^` + regexp.QuoteMeta(BackgroundPath) + `(?:\?v=([0-9a-f]{64}))?$`)
)

// Palette is an optional custom color palette. When present it overrides the
// legacy accent preset; when nil clients use the default/preset behavior.
type Palette struct {
	Primary    string `json:"primary"`
	Secondary  string `json:"secondary"`
	Accent     string `json:"accent"`
	Background string `json:"background"`
}

// Validate requires each color to be a strict #RRGGBB hex value.
func (p Palette) Validate() error {
	for _, c := range []struct{ name, value string }{
		{"primary", p.Primary}, {"secondary", p.Secondary}, {"accent", p.Accent}, {"background", p.Background},
	} {
		if !hexColorPattern.MatchString(c.value) {
			return fmt.Errorf("palette.%s must be a #RRGGBB hex color", c.name)
		}
	}
	return nil
}

// Settings is the complete appearance configuration shared by all users.
type Settings struct {
	Theme         string `json:"theme"`
	Accent        string `json:"accent"`
	BackgroundURL string `json:"backgroundUrl"`
	BackgroundDim int    `json:"backgroundDim"`
	// Palette is nil when no custom palette is configured. It is a pointer so
	// legacy settings decode unchanged and are omitted from responses.
	Palette *Palette `json:"palette,omitempty"`
}

// InternalBackground reports whether url references the uploaded background
// image, returning the optional content hash from its ?v= query.
func InternalBackground(url string) (hash string, ok bool) {
	m := internalBackgroundExpr.FindStringSubmatch(url)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// Defaults returns the appearance of an instance with no saved settings.
func Defaults() Settings {
	return Settings{Theme: "system", Accent: "default", BackgroundDim: 70}
}

// Validate checks the supported controls. Background images are loaded only by
// clients; the server never fetches the configured URL.
func (s Settings) Validate() error {
	switch s.Theme {
	case "system", "light", "dark", "amoled":
	default:
		return fmt.Errorf("theme must be system, light, dark, or amoled")
	}
	switch s.Accent {
	case "default", "teal", "blue", "rose", "amber":
	default:
		return fmt.Errorf("accent must be default, teal, blue, rose, or amber")
	}
	if s.BackgroundDim < 0 || s.BackgroundDim > 100 {
		return fmt.Errorf("backgroundDim must be an integer between 0 and 100")
	}
	if len(s.BackgroundURL) > 2048 {
		return fmt.Errorf("backgroundUrl must not exceed 2048 bytes")
	}
	if s.Palette != nil {
		if err := s.Palette.Validate(); err != nil {
			return err
		}
	}
	if _, internal := InternalBackground(s.BackgroundURL); s.BackgroundURL != "" && !internal {
		u, err := url.Parse(s.BackgroundURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
			return fmt.Errorf("backgroundUrl must be empty, an absolute http/https URL without credentials, or " + BackgroundPath + " with an optional ?v=<sha256> query")
		}
	}
	return nil
}
