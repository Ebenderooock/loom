package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// musicBrainzUserAgent identifies Loom to the MusicBrainz API, as required by
// their usage policy. MusicBrainz blocks requests without a descriptive agent.
const musicBrainzUserAgent = "Loom/1.0 (https://github.com/Ebenderooock/loom)"

// MusicBrainzProvider imports artists from a public MusicBrainz collection. The
// collection MBID is supplied via the list URL (full collection URL or bare
// MBID). It is keyless and respects the 1 req/s policy by issuing a single
// paged request sequence per sync.
type MusicBrainzProvider struct {
	client  *http.Client
	baseURL string
}

// NewMusicBrainz returns a MusicBrainz collection import-list provider.
func NewMusicBrainz() *MusicBrainzProvider {
	return &MusicBrainzProvider{
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: "https://musicbrainz.org/ws/2",
	}
}

type mbArtistList struct {
	Artists []struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Disambiguation string `json:"disambiguation"`
	} `json:"artists"`
	ArtistCount int `json:"artist-count"`
}

// Fetch retrieves the artists in the configured MusicBrainz collection.
func (p *MusicBrainzProvider) Fetch(ctx context.Context, cfg ProviderConfig) ([]Item, error) {
	mbid := extractCollectionMBID(cfg.URL)
	if mbid == "" {
		return nil, fmt.Errorf("musicbrainz: a collection MBID (or URL) is required")
	}

	var items []Item
	const pageSize = 100
	offset := 0
	for {
		list, err := p.fetchPage(ctx, mbid, offset, pageSize)
		if err != nil {
			return nil, err
		}
		for _, a := range list.Artists {
			if a.ID == "" {
				continue
			}
			items = append(items, Item{
				ExternalID: a.ID,
				Title:      a.Name,
				MediaType:  "music",
			})
		}
		offset += pageSize
		if offset >= list.ArtistCount || len(list.Artists) == 0 {
			break
		}
		// Respect the MusicBrainz 1 req/s policy between pages.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return items, nil
}

func (p *MusicBrainzProvider) fetchPage(ctx context.Context, collectionMBID string, offset, limit int) (*mbArtistList, error) {
	u := fmt.Sprintf("%s/artist?collection=%s&fmt=json&limit=%d&offset=%d",
		p.baseURL, url.QueryEscape(collectionMBID), limit, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz: build request: %w", err)
	}
	req.Header.Set("User-Agent", musicBrainzUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musicbrainz: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("musicbrainz: read body: %w", err)
	}
	var list mbArtistList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("musicbrainz: decode: %w", err)
	}
	return &list, nil
}

// extractCollectionMBID accepts either a bare MBID or a MusicBrainz collection
// URL and returns the collection MBID.
func extractCollectionMBID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, "/collection/"); i >= 0 {
		rest := s[i+len("/collection/"):]
		if j := strings.IndexAny(rest, "/?#"); j >= 0 {
			rest = rest[:j]
		}
		return rest
	}
	return s
}
