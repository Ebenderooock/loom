package series

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/metadata/tvmaze"
)

func TestMatchingAiringTime(t *testing.T) {
	base := tvmaze.Episode{Season: 1, Number: 1, Name: "The Pilot!", AirDate: "2026-10-21", AirTime: "20:00", AirStamp: "2026-10-22T00:00:00+00:00"}
	ep := &Episode{EpisodeNumber: 1, Title: "The pilot", AirDate: "2026-10-21"}
	tests := []struct {
		name   string
		change func(*tvmaze.Episode)
	}{
		{"number mismatch", func(r *tvmaze.Episode) { r.Number = 2 }},
		{"anime order mismatch", func(r *tvmaze.Episode) { r.Season = 2 }},
		{"date mismatch", func(r *tvmaze.Episode) { r.AirDate = "2026-10-22" }},
		{"title mismatch", func(r *tvmaze.Episode) { r.Name = "Unrelated episode" }},
		{"missing time", func(r *tvmaze.Episode) { r.AirTime = "" }},
		{"invalid time", func(r *tvmaze.Episode) { r.AirTime = "25:00" }},
		{"date-only stamp", func(r *tvmaze.Episode) { r.AirStamp = "2026-10-21" }},
		{"offset required", func(r *tvmaze.Episode) { r.AirStamp = "2026-10-21T20:00:00" }},
	}
	if got := matchingAiringTime(ep, 1, []tvmaze.Episode{base}); got != "2026-10-22T00:00:00Z" {
		t.Fatalf("UTC airstamp must be used directly: %s", got)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record := base
			tc.change(&record)
			if got := matchingAiringTime(ep, 1, []tvmaze.Episode{record}); got != "" {
				t.Fatalf("unsafe match: %s", got)
			}
		})
	}
	if got := matchingAiringTime(ep, 0, []tvmaze.Episode{base}); got != "" {
		t.Fatal("special must not match")
	}
	if got := matchingAiringTime(ep, 1, []tvmaze.Episode{base, base}); got != "" {
		t.Fatal("ambiguous match must be rejected")
	}
	base.AirTime = "00:00"
	base.AirStamp = "2026-10-21T00:00:00-04:00"
	if got := matchingAiringTime(ep, 1, []tvmaze.Episode{base}); got != "2026-10-21T04:00:00Z" {
		t.Fatalf("explicit midnight is a real time: %s", got)
	}
}

type fakeAiringProvider struct {
	records []tvmaze.Episode
	err     error
}

func (f fakeAiringProvider) Episodes(context.Context, string, string) ([]tvmaze.Episode, error) {
	return f.records, f.err
}

func TestEnrichAiringTimesAndRefreshFallback(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	repo.seasons["show"] = []*Season{{ID: "season", SeasonNumber: 1}}
	repo.episodes["show"] = []*Episode{{
		ID: "old", SeriesID: "show", SeasonID: "season", EpisodeNumber: 1,
		Title: "Pilot", AirDate: "2026-10-21", Monitored: false, HasFile: true,
	}}
	svc := NewService(repo, "", WithAiringProvider(fakeAiringProvider{records: []tvmaze.Episode{{
		Season: 1, Number: 1, Name: "Pilot", AirDate: "2026-10-21", AirTime: "20:00", AirStamp: "2026-10-21T20:00:00-04:00",
	}}})).(*service)
	svc.enrichAiringTimes(ctx, &Series{ID: "show"}, nil)
	ep := repo.episodes["show"][0]
	if ep.AirDateUTC != "2026-10-22T00:00:00Z" || ep.AirDate != "2026-10-21" || ep.Monitored || !ep.HasFile {
		t.Fatalf("enrichment changed unrelated state: %+v", ep)
	}

	saved := svc.savedAiringTimes(ctx, "show")
	// Simulate refresh recreation with new IDs.
	repo.seasons["show"][0].ID = "new-season"
	ep.ID, ep.SeasonID, ep.AirDateUTC = "new", "new-season", ""
	svc.airingProvider = fakeAiringProvider{err: errors.New("optional provider offline")}
	svc.enrichAiringTimes(ctx, &Series{ID: "show"}, saved)
	if ep.AirDateUTC != "2026-10-22T00:00:00Z" {
		t.Fatal("lost verified time during optional provider failure")
	}
	ep.AirDate, ep.AirDateUTC = "2026-10-28", ""
	svc.enrichAiringTimes(ctx, &Series{ID: "show"}, saved)
	if ep.AirDateUTC != "" {
		t.Fatal("stale timestamp attached to rescheduled episode")
	}
}

type airingFixtureTransport func(*http.Request) (*http.Response, error)

func (f airingFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestAddAndRefreshEnrichAiringTimes(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	provider := fakeAiringProvider{records: []tvmaze.Episode{{
		Season: 1, Number: 1, Name: "Pilot", AirDate: "2026-10-21",
		AirTime: "20:00", AirStamp: "2026-10-22T00:00:00Z",
	}}}
	svc := NewService(repo, "", WithAiringProvider(provider)).(*service)
	svc.httpClient = &http.Client{Transport: airingFixtureTransport(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Path {
		case "/3/tv/123":
			body = `{"name":"Test Show","first_air_date":"2026-10-21","external_ids":{"tvdb_id":42},"seasons":[{"season_number":1,"name":"Season 1","episode_count":1}]}`
		case "/3/tv/123/season/1":
			body = `{"episodes":[{"episode_number":1,"name":"Pilot","air_date":"2026-10-21"}]}`
		default:
			t.Fatalf("unexpected metadata request: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	sr, err := svc.AddSeries(ctx, &AddSeriesRequest{TMDBID: "123"})
	if err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		episodes, err := repo.ListEpisodes(ctx, sr.ID, nil)
		if err != nil || len(episodes) != 1 {
			t.Fatalf("episodes=%+v, err=%v", episodes, err)
		}
		response := episodeToResponse(episodes[0])
		if response["airDate"] != "2026-10-21" || response["airDateUtc"] != "2026-10-22T00:00:00Z" {
			t.Fatalf("native response: %+v", response)
		}
	}
	check()
	// Optional provider failure must not fail refresh or erase verified times.
	svc.airingProvider = fakeAiringProvider{err: errors.New("offline")}
	if err := svc.RefreshSeries(ctx, sr.ID); err != nil {
		t.Fatal(err)
	}
	check()
	// A successful later refresh can update the scheduled instant.
	provider.records[0].AirStamp = "2026-10-22T01:00:00Z"
	svc.airingProvider = provider
	if err := svc.RefreshSeries(ctx, sr.ID); err != nil {
		t.Fatal(err)
	}
	if got := repo.episodes[sr.ID][0].AirDateUTC; got != "2026-10-22T01:00:00Z" {
		t.Fatalf("refresh did not update timestamp: %s", got)
	}
}
