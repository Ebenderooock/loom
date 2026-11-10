package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/ebenderooock/loom/internal/series"
)

type researchMonitoringSeries struct {
	series.Service
	status          series.MonitoringStatus
	seasonMonitored bool
}

func (s researchMonitoringSeries) GetEpisode(context.Context, string) (*series.Episode, error) {
	return &series.Episode{ID: "episode", SeriesID: "show", SeasonID: "season", EpisodeNumber: 1, Monitored: true}, nil
}

func (s researchMonitoringSeries) GetSeries(context.Context, string) (*series.Series, error) {
	return &series.Series{ID: "show", MonitoringStatus: s.status}, nil
}

func (s researchMonitoringSeries) ListSeasons(context.Context, string) ([]*series.Season, error) {
	return []*series.Season{{ID: "season", SeriesID: "show", SeasonNumber: 1, Monitored: s.seasonMonitored}}, nil
}

func TestAbandonedDownloadsDoNotResearchUnmonitoredShowsOrSeasons(t *testing.T) {
	for _, tc := range []struct {
		status          series.MonitoringStatus
		seasonMonitored bool
	}{
		{series.MonitoringNone, true},
		{series.MonitoringUnmonitored, true},
		{series.MonitoringArchived, true},
		{series.MonitoringAll, false},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			// A nil engine must never be reached for unmonitored media.
			research := newResearchFunc(nil, nil, researchMonitoringSeries{
				status: tc.status, seasonMonitored: tc.seasonMonitored,
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			research(context.Background(), "episode", []string{"episode"})
		})
	}
}
