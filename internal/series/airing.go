package series

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/ebenderooock/loom/internal/metadata/tvmaze"
)

// AiringProvider supplies optional actual broadcast times for a known series.
type AiringProvider interface {
	Episodes(ctx context.Context, tvdbID, imdbID string) ([]tvmaze.Episode, error)
}

// WithAiringProvider enables optional broadcast-time enrichment on add/refresh.
func WithAiringProvider(p AiringProvider) Option {
	return func(s *service) { s.airingProvider = p }
}

func airingTitle(title string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, title)
}

type airingKey struct {
	season, episode int
	date, title     string
}

func episodeAiringKey(ep *Episode, season int) airingKey {
	return airingKey{season, ep.EpisodeNumber, ep.AirDate, airingTitle(ep.Title)}
}

// Refresh recreates episode IDs. Preserve known instants only for the same
// identity if optional enrichment is unavailable, never across date/order changes.
func (s *service) savedAiringTimes(ctx context.Context, seriesID string) map[airingKey]string {
	saved := make(map[airingKey]string)
	seasons, err := s.repo.ListSeasons(ctx, seriesID)
	if err != nil {
		slog.Warn("series: cannot preserve airing times", "series", seriesID, "error", err)
		return saved
	}
	episodes, err := s.repo.ListEpisodes(ctx, seriesID, nil)
	if err != nil {
		slog.Warn("series: cannot preserve airing times", "series", seriesID, "error", err)
		return saved
	}
	numbers := make(map[string]int)
	for _, season := range seasons {
		numbers[season.ID] = season.SeasonNumber
	}
	for _, ep := range episodes {
		if ep.AirDateUTC != "" {
			saved[episodeAiringKey(ep, numbers[ep.SeasonID])] = ep.AirDateUTC
		}
	}
	return saved
}

func (s *service) enrichAiringTimes(ctx context.Context, sr *Series, saved map[airingKey]string) {
	tvdbID, imdbID := "", ""
	if sr.TVDBID != nil {
		tvdbID = *sr.TVDBID
	}
	if sr.IMDBID != nil {
		imdbID = *sr.IMDBID
	}
	var records []tvmaze.Episode
	if s.airingProvider != nil {
		var err error
		records, err = s.airingProvider.Episodes(ctx, tvdbID, imdbID)
		if err != nil {
			slog.Warn("series: optional airing metadata unavailable", "series", sr.ID, "error", err)
			records = nil
		}
	}
	if len(records) == 0 && len(saved) == 0 {
		return
	}
	seasons, err := s.repo.ListSeasons(ctx, sr.ID)
	if err != nil {
		slog.Warn("series: airing metadata seasons lookup failed", "series", sr.ID, "error", err)
		return
	}
	episodes, err := s.repo.ListEpisodes(ctx, sr.ID, nil)
	if err != nil {
		slog.Warn("series: airing metadata episodes lookup failed", "series", sr.ID, "error", err)
		return
	}
	numbers := make(map[string]int)
	for _, season := range seasons {
		numbers[season.ID] = season.SeasonNumber
	}
	for _, ep := range episodes {
		instant := matchingAiringTime(ep, numbers[ep.SeasonID], records)
		if len(records) == 0 {
			instant = saved[episodeAiringKey(ep, numbers[ep.SeasonID])]
		}
		if instant == "" {
			continue
		}
		ep.AirDateUTC = instant
		ep.UpdatedAt = now()
		if err := s.repo.UpdateEpisodeAiringTime(ctx, ep); err != nil {
			slog.Warn("series: airing metadata persistence failed", "series", sr.ID, "error", err)
		}
	}
}

// Numbering alone is unsafe across providers, particularly for anime. Require
// agreement on season, number, date and nonempty normalized title, and reject
// ambiguous matches. Specials have incompatible provider numbering: skip them.
func matchingAiringTime(ep *Episode, season int, records []tvmaze.Episode) string {
	if season <= 0 || ep.EpisodeNumber <= 0 || ep.AirDate == "" || airingTitle(ep.Title) == "" {
		return ""
	}
	match := ""
	count := 0
	for _, r := range records {
		if r.Season != season || r.Number != ep.EpisodeNumber || r.AirDate != ep.AirDate || airingTitle(r.Name) != airingTitle(ep.Title) {
			continue
		}
		count++
		// TVmaze can supply a midnight airstamp when airtime is unknown.
		// A timestamp by itself is not evidence of an actual broadcast time.
		if _, err := time.Parse("15:04", r.AirTime); err != nil {
			continue
		}
		// airstamp may already be UTC while airdate/airtime describe the
		// network's local schedule; do not compare their wall-clock fields.
		if t, err := time.Parse(time.RFC3339, r.AirStamp); err == nil {
			match = t.UTC().Format(time.RFC3339)
		}
	}
	if count != 1 {
		return ""
	}
	return match
}
