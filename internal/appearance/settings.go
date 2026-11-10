package appearance

import (
	"fmt"
	"net/url"
)

// Settings is the complete appearance configuration shared by all users.
type Settings struct {
	Theme         string `json:"theme"`
	Accent        string `json:"accent"`
	BackgroundURL string `json:"backgroundUrl"`
	BackgroundDim int    `json:"backgroundDim"`
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
	if s.BackgroundURL != "" {
		u, err := url.Parse(s.BackgroundURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
			return fmt.Errorf("backgroundUrl must be empty or an absolute http/https URL without credentials")
		}
	}
	return nil
}
