package prowlarrv1

import (
	"encoding/json"
	"strings"

	"github.com/loomctl/loom/internal/indexers"
)

// categoryName returns a human-readable label for well-known Newznab
// category families.
func categoryName(c indexers.Category) string {
	switch {
	case c >= 1000 && c < 2000:
		return "Console"
	case c >= 2000 && c < 3000:
		return "Movies"
	case c >= 3000 && c < 4000:
		return "Audio"
	case c >= 4000 && c < 5000:
		return "PC"
	case c >= 5000 && c < 6000:
		return "TV"
	case c >= 6000 && c < 7000:
		return "XXX"
	case c >= 7000 && c < 8000:
		return "Books"
	default:
		return "Other"
	}
}

// protocolFromKind infers a Prowlarr protocol string from the
// Definition's Kind. Falls back to "torrent" for unknown kinds.
func protocolFromKind(k indexers.Kind) string {
	s := strings.ToLower(string(k))
	if strings.Contains(s, "newznab") || strings.Contains(s, "usenet") || strings.Contains(s, "nzb") {
		return "usenet"
	}
	return "torrent"
}

// implFromKind returns Prowlarr-style implementation and
// configContract strings.
func implFromKind(k indexers.Kind) (implName, impl, contract string) {
	s := strings.ToLower(string(k))
	switch {
	case strings.Contains(s, "newznab"):
		return "Newznab", "Newznab", "NewznabSettings"
	case strings.Contains(s, "torznab"):
		return "Torznab", "Torznab", "TorznabSettings"
	case strings.Contains(s, "cardigann"):
		return "Cardigann", "Cardigann", "CardigannSettings"
	default:
		return "Generic", "Generic", "GenericSettings"
	}
}

// configURL extracts a baseUrl from the Definition's Config JSON.
func configURL(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, key := range []string{"base_url", "baseUrl", "url", "siteUrl"} {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// defToIndexer converts a Loom Definition into a Prowlarr v1 indexer
// JSON object.
func defToIndexer(d indexers.Definition) prowlarrIndexer {
	implName, impl, contract := implFromKind(d.Kind)
	protocol := protocolFromKind(d.Kind)

	fields := []prowlarrField{
		{Name: "baseUrl", Value: configURL(d.Config)},
	}

	tags := make([]int, 0)

	return prowlarrIndexer{
		ID:                 intID(d.ID),
		Name:               d.Name,
		Protocol:           protocol,
		Enable:             d.Enabled,
		Priority:           d.Priority,
		AppProfileID:       1,
		Fields:             fields,
		ImplementationName: implName,
		Implementation:     impl,
		ConfigContract:     contract,
		Tags:               tags,
	}
}

// resultToSearch converts a Loom search Result into a Prowlarr v1
// search result, attaching the indexer's numeric ID and protocol.
func resultToSearch(r indexers.Result, numericIndexerID int, protocol string) prowlarrSearchResult {
	cats := make([]prowlarrCategory, 0, len(r.Category))
	for _, c := range r.Category {
		cats = append(cats, prowlarrCategory{
			ID:   int(c),
			Name: categoryName(c),
		})
	}

	var leechers *int
	if r.Peers != nil && r.Seeders != nil {
		l := *r.Peers - *r.Seeders
		if l < 0 {
			l = 0
		}
		leechers = &l
	}

	sortTitle := strings.ToLower(r.Title)
	if idx := strings.IndexByte(sortTitle, '.'); idx > 0 {
		sortTitle = sortTitle[:idx]
	}

	return prowlarrSearchResult{
		GUID:        r.GUID,
		IndexerID:   numericIndexerID,
		Title:       r.Title,
		SortTitle:   sortTitle,
		Size:        r.Size,
		PublishDate: r.PubDate.UTC().Format("2006-01-02T15:04:05Z"),
		DownloadURL: r.Link,
		InfoURL:     r.InfoURL,
		Categories:  cats,
		Protocol:    protocol,
		Seeders:     r.Seeders,
		Leechers:    leechers,
		MagnetURL:   r.MagnetURI,
	}
}
