package main

import (
	"context"
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/metadata/tvdb"
	"github.com/ebenderooock/loom/internal/series"
)

// tvdbEpisodeProvider adapts the TVDB client to series.EpisodeProvider so anime
// series can be segmented using TVDB aired-order numbering.
type tvdbEpisodeProvider struct {
	client     *tvdb.Client
	seasonType string
}

// ResolveSeriesID resolves a TVDB series ID from external IDs (preferring a
// known TVDB id) or, failing that, a title + year search with a confident match.
func (p *tvdbEpisodeProvider) ResolveSeriesID(ctx context.Context, title string, year int, externalIDs map[string]string) (int, error) {
	results, err := p.client.FindSeries(ctx, title, externalIDs)
	if err != nil {
		return 0, err
	}

	want := normalizeTitle(title)
	best := 0
	bestScore := 0
	for _, r := range results {
		if r == nil || r.TVDBID == nil {
			continue
		}
		id, convErr := strconv.Atoi(*r.TVDBID)
		if convErr != nil || id <= 0 {
			continue
		}
		score := 0
		if normalizeTitle(r.Title) == want {
			score += 2
		}
		if year > 0 {
			if ry := firstAirYear(r.FirstAirDate); ry == year {
				score += 2
			} else if ry != 0 {
				// Year known but mismatched: penalize so we don't pick a
				// different series with the same name.
				score--
			}
		}
		if score > bestScore {
			bestScore = score
			best = id
		}
	}

	// Require a confident match (title or year agreement) to avoid mis-mapping.
	if bestScore <= 0 {
		return 0, nil
	}
	return best, nil
}

// SeriesEpisodes returns the series' episodes in the configured season type.
func (p *tvdbEpisodeProvider) SeriesEpisodes(ctx context.Context, providerSeriesID int) ([]series.ProviderEpisode, error) {
	recs, err := p.client.GetSeriesEpisodes(ctx, providerSeriesID, p.seasonType)
	if err != nil {
		return nil, err
	}
	out := make([]series.ProviderEpisode, 0, len(recs))
	for _, r := range recs {
		out = append(out, series.ProviderEpisode{
			SeasonNumber:   r.SeasonNumber,
			EpisodeNumber:  r.Number,
			AbsoluteNumber: r.AbsoluteNumber,
			Runtime:        r.Runtime,
			Title:          r.Name,
			Overview:       r.Overview,
			AirDate:        r.Aired,
			StillPath:      r.Image,
		})
	}
	return out, nil
}

func normalizeTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstAirYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
}
