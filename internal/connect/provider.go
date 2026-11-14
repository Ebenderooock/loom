package connect

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Provider defines the interface for media server integrations.
type Provider interface {
	Test(ctx context.Context, settings ProviderSettings) error
	NotifyLibraryUpdate(ctx context.Context, settings ProviderSettings) error
	// ActiveSessions returns the playback sessions currently active on the
	// server. Providers that don't stream (e.g. Trakt) return nil, nil.
	ActiveSessions(ctx context.Context, settings ProviderSettings) ([]Session, error)
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// ProviderFor returns the provider implementation for the given type.
func ProviderFor(t ProviderType) (Provider, error) {
	switch t {
	case ProviderPlex:
		return &plexProvider{}, nil
	case ProviderEmby:
		return &embyProvider{}, nil
	case ProviderJellyfin:
		return &jellyfinProvider{}, nil
	case ProviderTrakt:
		return &traktProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", t)
	}
}

// ---------- Plex ----------

type plexProvider struct{}

func (p *plexProvider) Test(ctx context.Context, s ProviderSettings) error {
	resp, err := plexGet(ctx, s, "/identity", "application/json", "test")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("plex test: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func plexGet(ctx context.Context, s ProviderSettings, path, accept, operation string) (*http.Response, error) {
	base, err := url.Parse(s.Host)
	if err != nil || base.Hostname() == "" || (base.Scheme != "http" && base.Scheme != "https") ||
		base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" {
		return nil, fmt.Errorf("plex config: host must be an absolute HTTP(S) server URL without credentials, query, or fragment")
	}
	if s.APIKey == "" || strings.IndexFunc(s.APIKey, func(r rune) bool { return r <= ' ' || r == 127 }) >= 0 {
		return nil, fmt.Errorf("plex config: api_key must contain a valid Plex token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.Host, "/")+path, nil)
	if err != nil {
		return nil, fmt.Errorf("plex %s: invalid request URL", operation)
	}
	req.Header.Set("X-Plex-Token", s.APIKey)
	req.Header.Set("Accept", accept)

	// Plex tokens are custom headers that net/http otherwise forwards on redirects.
	client := *httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("plex %s: %w", operation, err)
	}
	return resp, nil
}

// plexMediaContainer is used to parse the /library/sections response.
type plexMediaContainer struct {
	XMLName   xml.Name  `xml:"MediaContainer"`
	Directory []plexDir `xml:"Directory"`
}

type plexDir struct {
	Key string `xml:"key,attr"`
}

func (p *plexProvider) NotifyLibraryUpdate(ctx context.Context, s ProviderSettings) error {
	// List library sections.
	resp, err := plexGet(ctx, s, "/library/sections", "application/xml", "sections")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("plex sections: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("plex sections read: %w", err)
	}

	var mc plexMediaContainer
	if err := xml.Unmarshal(body, &mc); err != nil {
		return fmt.Errorf("plex sections parse: %s", strings.ReplaceAll(err.Error(), s.APIKey, "[redacted]"))
	}
	if len(mc.Directory) == 0 {
		return fmt.Errorf("plex sections: no library sections returned")
	}
	for _, dir := range mc.Directory {
		id, err := strconv.ParseUint(dir.Key, 10, 64)
		if err != nil || id == 0 {
			return fmt.Errorf("plex sections: invalid library section ID")
		}
	}

	// Refresh each section.
	refreshed := make(map[string]bool)
	for _, dir := range mc.Directory {
		if refreshed[dir.Key] {
			continue
		}
		rResp, err := plexGet(ctx, s, "/library/sections/"+dir.Key+"/refresh", "application/xml", "refresh "+dir.Key)
		if err != nil {
			return err
		}
		rResp.Body.Close()
		if rResp.StatusCode < 200 || rResp.StatusCode >= 300 {
			return fmt.Errorf("plex refresh %s: unexpected status %d", dir.Key, rResp.StatusCode)
		}
		refreshed[dir.Key] = true
	}

	return nil
}

// ---------- Emby ----------

type embyProvider struct{}

func (p *embyProvider) Test(ctx context.Context, s ProviderSettings) error {
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(s.Host, "/")+"/System/Info", nil)
	if err != nil {
		return fmt.Errorf("emby test: %w", err)
	}
	req.Header.Set("X-Emby-Token", s.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("emby test: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("emby test: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (p *embyProvider) NotifyLibraryUpdate(ctx context.Context, s ProviderSettings) error {
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(s.Host, "/")+"/Library/Refresh", nil)
	if err != nil {
		return fmt.Errorf("emby refresh: %w", err)
	}
	req.Header.Set("X-Emby-Token", s.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("emby refresh: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("emby refresh: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ---------- Jellyfin ----------

type jellyfinProvider struct{}

func (p *jellyfinProvider) Test(ctx context.Context, s ProviderSettings) error {
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(s.Host, "/")+"/System/Info", nil)
	if err != nil {
		return fmt.Errorf("jellyfin test: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, s.APIKey))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jellyfin test: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jellyfin test: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (p *jellyfinProvider) NotifyLibraryUpdate(ctx context.Context, s ProviderSettings) error {
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(s.Host, "/")+"/Library/Refresh", nil)
	if err != nil {
		return fmt.Errorf("jellyfin refresh: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, s.APIKey))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jellyfin refresh: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("jellyfin refresh: unexpected status %d", resp.StatusCode)
	}
	return nil
}
