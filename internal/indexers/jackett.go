package indexers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// JackettIndexer is a discovered indexer from a Jackett instance.
type JackettIndexer struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Language   string `json:"language,omitempty"`
	SiteLink   string `json:"site_link,omitempty"`
	Configured bool   `json:"configured"`
}

// JackettImportResult is the result of importing from Jackett.
type JackettImportResult struct {
	Imported []JackettIndexer `json:"imported"`
	Skipped  []JackettIndexer `json:"skipped"`
	Errors   []string         `json:"errors"`
}

// jackettConfiguredIndexer matches Jackett's /api/v2.0/indexers response.
type jackettConfiguredIndexer struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Language   string `json:"language"`
	SiteLink   string `json:"site_link"`
	Configured bool   `json:"configured"`
}

// ImportFromJackett discovers configured indexers from a Jackett instance
// and creates Loom indexer definitions for each.
func ImportFromJackett(ctx context.Context, jackettURL, apiKey string, svc *Service) (*JackettImportResult, error) {
	if jackettURL == "" || apiKey == "" {
		return nil, fmt.Errorf("jackett_url and api_key are required")
	}

	jackettURL = strings.TrimRight(jackettURL, "/")
	url := fmt.Sprintf("%s/api/v2.0/indexers?apikey=%s", jackettURL, apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch jackett indexers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jackett returned %d: %s", resp.StatusCode, string(body))
	}

	var indexers []jackettConfiguredIndexer
	if err := json.NewDecoder(resp.Body).Decode(&indexers); err != nil {
		return nil, fmt.Errorf("decode jackett response: %w", err)
	}

	result := &JackettImportResult{}

	for _, jix := range indexers {
		if !jix.Configured {
			result.Skipped = append(result.Skipped, JackettIndexer{
				ID:   jix.ID,
				Name: jix.Name,
				Type: jix.Type,
			})
			continue
		}

		kind := Kind("torznab")
		if strings.Contains(strings.ToLower(jix.Type), "usenet") || strings.Contains(strings.ToLower(jix.Type), "newznab") {
			kind = Kind("newznab")
		}

		loomID := "jackett-" + jix.ID

		config := map[string]any{
			"url":     fmt.Sprintf("%s/api/v2.0/indexers/%s/results/torznab/", jackettURL, jix.ID),
			"api_key": apiKey,
		}
		if kind == Kind("newznab") {
			config["url"] = fmt.Sprintf("%s/api/v2.0/indexers/%s/results/", jackettURL, jix.ID)
		}

		configJSON, _ := json.Marshal(config)

		def := Definition{
			ID:       loomID,
			Kind:     kind,
			Name:     fmt.Sprintf("Jackett - %s", jix.Name),
			Enabled:  true,
			Priority: 25,
			Config:   configJSON,
		}

		if svc != nil {
			if _, err := svc.Create(ctx, def); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", jix.Name, err.Error()))
				continue
			}
		}

		result.Imported = append(result.Imported, JackettIndexer{
			ID:         loomID,
			Name:       jix.Name,
			Type:       string(kind),
			Language:   jix.Language,
			SiteLink:   jix.SiteLink,
			Configured: true,
		})
	}

	return result, nil
}
