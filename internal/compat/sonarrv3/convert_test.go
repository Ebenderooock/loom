package sonarrv3

import (
	"encoding/json"
	"testing"

	"github.com/ebenderooock/loom/internal/series"
)

func TestSeriesCompletenessExcludesSpecials(t *testing.T) {
	show := &series.Series{
		ID: "show", MonitoringStatus: series.MonitoringAll,
		Seasons: []*series.Season{
			{ID: "specials", SeasonNumber: 0, Monitored: true},
			{ID: "regular", SeasonNumber: 1, Monitored: true},
		},
		Episodes: []*series.Episode{
			{ID: "first", SeasonID: "regular", HasFile: true},
			{ID: "bonus", SeasonID: "specials"},
			{ID: "bonus-file", SeasonID: "specials", HasFile: true},
		},
	}
	got := seriesToSonarr(show, newIDCache(), nil)
	if !got.Monitored || len(got.Seasons) != 2 || !got.Seasons[0].Monitored {
		t.Fatalf("lost show/specials monitoring: %+v", got)
	}
	if got.Statistics.EpisodeCount != 1 || got.Statistics.TotalEpisodeCount != 1 ||
		got.Statistics.EpisodeFileCount != 1 || got.Statistics.PercentOfEpisodes != 100 {
		t.Fatalf("specials changed show completion: %+v", got.Statistics)
	}
	show.MonitoringStatus = series.MonitoringArchived
	if got := seriesToSonarr(show, newIDCache(), nil); got.Monitored {
		t.Fatal("archived show must not be monitored")
	}
}

func TestEpisodeAiringTimestamp(t *testing.T) {
	ep := &series.Episode{ID: "ep", SeriesID: "show", EpisodeNumber: 1, AirDate: "2026-10-21", AirDateUTC: "2026-10-22T00:00:00Z"}
	converted := episodeToSonarr(ep, 1, newIDCache())
	body, err := json.Marshal(converted)
	if err != nil {
		t.Fatal(err)
	}

	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response["airDate"] != ep.AirDate || response["airDateUtc"] != ep.AirDateUTC {
		t.Fatalf("lost date/instant distinction: %s", body)
	}
	ep.AirDateUTC = ""
	body, err = json.Marshal(episodeToSonarr(ep, 1, newIDCache()))
	if err != nil {
		t.Fatal(err)
	}
	response = nil
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if _, ok := response["airDateUtc"]; ok {
		t.Fatalf("date-only metadata must not gain a guessed timestamp: %s", body)
	}
}
