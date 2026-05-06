package connect

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider defines the interface for media server integrations.
type Provider interface {
	Test(ctx context.Context, settings ProviderSettings) error
	NotifyLibraryUpdate(ctx context.Context, settings ProviderSettings) error
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
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(s.Host, "/")+"/identity", nil)
	if err != nil {
		return fmt.Errorf("plex test: %w", err)
	}
	req.Header.Set("X-Plex-Token", s.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("plex test: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("plex test: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// plexMediaContainer is used to parse the /library/sections response.
type plexMediaContainer struct {
	XMLName   xml.Name     `xml:"MediaContainer"`
	Directory []plexDir    `xml:"Directory"`
}

type plexDir struct {
	Key string `xml:"key,attr"`
}

func (p *plexProvider) NotifyLibraryUpdate(ctx context.Context, s ProviderSettings) error {
	base := strings.TrimRight(s.Host, "/")

	// List library sections.
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/library/sections", nil)
	if err != nil {
		return fmt.Errorf("plex sections: %w", err)
	}
	req.Header.Set("X-Plex-Token", s.APIKey)
	req.Header.Set("Accept", "application/xml")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("plex sections: %w", err)
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
		return fmt.Errorf("plex sections parse: %w", err)
	}

	// Refresh each section.
	for _, dir := range mc.Directory {
		refreshURL := fmt.Sprintf("%s/library/sections/%s/refresh", base, dir.Key)
		rReq, err := http.NewRequestWithContext(ctx, "GET", refreshURL, nil)
		if err != nil {
			return fmt.Errorf("plex refresh %s: %w", dir.Key, err)
		}
		rReq.Header.Set("X-Plex-Token", s.APIKey)

		rResp, err := httpClient.Do(rReq)
		if err != nil {
			return fmt.Errorf("plex refresh %s: %w", dir.Key, err)
		}
		rResp.Body.Close()
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
