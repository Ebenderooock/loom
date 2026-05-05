package series

// SeriesTypeMiniSeries extends the built-in series type enum.
const TypeMiniSeries SeriesType = "miniseries"

// SeriesClassification holds the auto-detected series classification.
type SeriesClassification struct {
	DetectedType       SeriesType `json:"detectedType"`
	IsMiniSeries       bool       `json:"isMiniSeries"`
	HasSpecials        bool       `json:"hasSpecials"`
	IsDailySeries      bool       `json:"isDailySeries"`
	SpecialCount       int        `json:"specialCount"`
	TotalSeasons       int        `json:"totalSeasons"`
	TotalEpisodes      int        `json:"totalEpisodes"`
	MonitorSpecials    bool       `json:"monitorSpecials"`
}

// ClassifySeries auto-detects series type based on metadata.
// Mini-series: 1 season with ≤13 episodes.
// Daily: detected from existing SeriesType or naming patterns.
// Specials: Season 0 episodes present.
func ClassifySeries(s *Series) SeriesClassification {
	c := SeriesClassification{
		DetectedType: s.SeriesType,
	}

	if s.Seasons != nil {
		c.TotalSeasons = len(s.Seasons)
		for _, sn := range s.Seasons {
			if sn.SeasonNumber == 0 {
				c.HasSpecials = true
				c.SpecialCount = sn.EpisodeCount
			}
		}
	}

	if s.Episodes != nil {
		c.TotalEpisodes = len(s.Episodes)
	}

	// Mini-series: single season (excluding specials) with ≤13 episodes
	regularSeasons := c.TotalSeasons
	if c.HasSpecials {
		regularSeasons--
	}
	if regularSeasons == 1 && c.TotalEpisodes <= 13 && c.TotalEpisodes > 0 {
		c.IsMiniSeries = true
		c.DetectedType = TypeMiniSeries
	}

	if s.SeriesType == TypeDaily {
		c.IsDailySeries = true
	}

	return c
}

// IsSpecialEpisode returns true if the episode belongs to season 0.
func IsSpecialEpisode(seasonNumber int) bool {
	return seasonNumber == 0
}

// DailySearchQuery builds a search string for daily-type series using
// the air date instead of S/E numbers.
func DailySearchQuery(seriesTitle string, airDate string) string {
	if airDate == "" {
		return seriesTitle
	}
	return seriesTitle + " " + airDate
}

// ShouldMonitorSpecials returns true if specials monitoring is enabled
// and the episode is a special.
func ShouldMonitorSpecials(monitorSpecials bool, seasonNumber int) bool {
	if seasonNumber == 0 {
		return monitorSpecials
	}
	return true // non-specials follow normal monitoring
}
