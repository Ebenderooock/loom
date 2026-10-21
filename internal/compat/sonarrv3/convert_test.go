package sonarrv3

import (
	"encoding/json"
	"testing"

	"github.com/ebenderooock/loom/internal/series"
)

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
