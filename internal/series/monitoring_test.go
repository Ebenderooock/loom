package series

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/scheduler"
	"github.com/ebenderooock/loom/internal/storage"
)

func monitoringDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "monitoring.db")},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO series(id, title, year, overview, monitoring_status) VALUES
			('show', 'Show', 2000, '', 'all'), ('other', 'Other', 2000, '', 'all');
		INSERT INTO seasons(id, series_id, season_number, monitored) VALUES
			('specials', 'show', 0, 0), ('first', 'show', 1, 1),
			('second', 'show', 2, 1), ('other-first', 'other', 1, 1);
		INSERT INTO episodes(id, series_id, season_id, episode_number, air_date, monitored, has_file) VALUES
			('bonus', 'show', 'specials', 1, '2000-01-01', 0, 0),
			('bonus-file', 'show', 'specials', 2, '2000-01-01', 0, 1),
			('first-ep', 'show', 'first', 1, '2000-01-01', 1, 1),
			('second-ep', 'show', 'second', 1, '2000-01-01', 1, 0),
			('other-ep', 'other', 'other-first', 1, '2000-01-01', 1, 0);
	`); err != nil {
		t.Fatal(err)
	}
	return db.DB()
}

func TestSeasonMonitoringPersistsAndControlsAutomaticSearch(t *testing.T) {
	ctx := context.Background()
	db := monitoringDB(t)
	repo := NewRepository(db)
	svc := NewService(repo, "")
	router := Router(svc)
	set := func(season, body string, status int) {
		t.Helper()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/show/seasons/"+season+"/monitoring", strings.NewReader(body)))
		if rec.Code != status {
			t.Fatalf("season %s: status=%d body=%s", season, rec.Code, rec.Body)
		}
		if status == http.StatusOK {
			var response struct {
				SeasonNumber int  `json:"seasonNumber"`
				Monitored    bool `json:"monitored"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Monitored != strings.Contains(body, "true") {
				t.Fatalf("incorrect monitoring response: %s", rec.Body)
			}
		}
	}
	set("2", `{"monitored":false}`, http.StatusOK)
	set("0", `{"monitored":true}`, http.StatusOK)

	// A new repository/service instance must observe the persisted state.
	reloaded, err := NewService(NewRepository(db), "").GetSeries(ctx, "show")
	if err != nil {
		t.Fatal(err)
	}
	for _, season := range reloaded.Seasons {
		if season.Monitored != (season.SeasonNumber != 2) {
			t.Fatalf("wrong season state: %+v", season)
		}
	}
	for _, ep := range reloaded.Episodes {
		if ep.Monitored != (ep.SeasonID != "second") {
			t.Fatalf("wrong episode state: %+v", ep)
		}
	}
	other, err := repo.GetEpisode(ctx, "other-ep")
	if err != nil || !other.Monitored {
		t.Fatalf("monitoring leaked across shows: %+v, %v", other, err)
	}
	store := scheduler.NewStore(db)
	assertCandidates := func(want ...string) {
		t.Helper()
		candidates, err := store.GetCandidates(ctx, "episode", 100, 1)
		if err != nil {
			t.Fatal(err)
		}
		ids := make(map[string]bool)
		for _, c := range candidates {
			ids[c.MediaID] = true
		}
		if len(ids) != len(want) {
			t.Fatalf("candidates=%v, want=%v", ids, want)
		}
		for _, id := range want {
			if !ids[id] {
				t.Fatalf("missing candidate %s: %v", id, ids)
			}
		}
		count, err := store.QueueSize(ctx, 1)
		if err != nil || count != len(want) {
			t.Fatalf("queue size=%d err=%v", count, err)
		}
	}
	assertCandidates("bonus", "other-ep")
	if err := svc.SetMonitoringStatus(ctx, "show", MonitoringUnmonitored); err != nil {
		t.Fatal(err)
	}
	assertCandidates("other-ep")
	if err := svc.SetMonitoringStatus(ctx, "show", MonitoringMonitored); err != nil {
		t.Fatal(err)
	}
	assertCandidates("bonus", "other-ep")
	// Season gating also handles stale/externally written episode flags.
	if _, err := db.Exec(`UPDATE episodes SET monitored=1 WHERE id='second-ep'`); err != nil {
		t.Fatal(err)
	}
	assertCandidates("bonus", "other-ep")
	set("2", `{"monitored":true}`, http.StatusOK)
	assertCandidates("bonus", "second-ep", "other-ep")
	for _, season := range []string{"-1", "invalid"} {
		set(season, `{"monitored":true}`, http.StatusBadRequest)
	}
	for _, body := range []string{`{}`, `{"monitored":null}`, `{"monitored":"yes"}`, `{`} {
		set("1", body, http.StatusBadRequest)
	}
	set("99", `{"monitored":true}`, http.StatusNotFound)
}

func TestShowMonitoringFilterIncludesEnabledStrategies(t *testing.T) {
	ctx := context.Background()
	db := monitoringDB(t)
	svc := NewService(NewRepository(db), "")
	router := Router(svc)
	for status := range validMonitoringStatuses {
		t.Run(string(status), func(t *testing.T) {
			if err := svc.SetMonitoringStatus(ctx, "show", status); err != nil {
				t.Fatal(err)
			}
			for _, filter := range []string{"true", "false"} {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?monitored="+filter, nil))
				if rec.Code != http.StatusOK {
					t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
				}
				var response struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, show := range response.Data {
					found = found || show.ID == "show"
				}
				enabled := status != MonitoringNone && status != MonitoringUnmonitored && status != MonitoringArchived
				if found != (enabled == (filter == "true")) {
					t.Fatalf("status=%s filter=%s found=%v", status, filter, found)
				}
			}
		})
	}
}

func TestSeasonMonitoringRollsBackOnEpisodeFailure(t *testing.T) {
	ctx := context.Background()
	db := monitoringDB(t)
	if _, err := db.Exec(`CREATE TRIGGER reject_episode_monitoring BEFORE UPDATE OF monitored ON episodes
		BEGIN SELECT RAISE(ABORT, 'episode update failed'); END;`); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	if _, err := repo.SetSeasonMonitoring(ctx, "show", 1, false); err == nil {
		t.Fatal("expected update failure")
	}
	season, err := repo.GetSeason(ctx, "first")
	if err != nil || !season.Monitored {
		t.Fatalf("partially persisted season monitoring: %+v, %v", season, err)
	}
	if _, err := repo.SetSeasonMonitoring(ctx, "missing", 1, false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing season, got %v", err)
	}
}

func TestCompletenessExcludesSpecialsButSpecialsKeepTheirOwnStats(t *testing.T) {
	ctx := context.Background()
	db := monitoringDB(t)
	repo := NewRepository(db)
	if _, err := db.Exec(`UPDATE episodes SET has_file=1 WHERE id='second-ep'`); err != nil {
		t.Fatal(err)
	}
	for _, monitored := range []bool{false, true} {
		if _, err := repo.SetSeasonMonitoring(ctx, "show", 0, monitored); err != nil {
			t.Fatal(err)
		}
		stats, err := repo.GetEpisodeStats(ctx, "show")
		if err != nil {
			t.Fatal(err)
		}
		if stats.TotalEpisodes != 2 || stats.DownloadedEpisodes != 2 || stats.AiredEpisodes != 2 || stats.MissingEpisodes != 0 || stats.MonitoredEpisodes != 2 {
			t.Fatalf("specials changed show completeness: %+v", stats)
		}
		all, err := repo.GetAllEpisodeStats(ctx)
		if err != nil || all["show"] == nil || *all["show"] != *stats {
			t.Fatalf("inconsistent list/detail stats: %+v err=%v", all, err)
		}
		seasons, err := repo.GetSeasonEpisodeStats(ctx, "show")
		if err != nil {
			t.Fatal(err)
		}
		specials := seasons["specials"]
		wantMissing := 0
		if monitored {
			wantMissing = 1
		}
		if specials.TotalEpisodes != 2 || specials.DownloadedEpisodes != 1 || specials.AiredEpisodes != 2 || specials.MissingEpisodes != wantMissing {
			t.Fatalf("incorrect specials stats: %+v", specials)
		}
	}
	if _, err := db.Exec(`DELETE FROM episodes WHERE season_id != 'specials'`); err != nil {
		t.Fatal(err)
	}
	stats, err := repo.GetEpisodeStats(ctx, "show")
	if err != nil || *stats != (EpisodeStats{}) {
		t.Fatalf("specials-only show should have zero regular stats: %+v, %v", stats, err)
	}
}

func TestRefreshPreservesSeasonAndEpisodeMonitoring(t *testing.T) {
	for _, anime := range []bool{false, true} {
		name := "TMDB"
		if anime {
			name = "TVDB"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := newMockRepo()
			svc := NewService(repo, "").(*service)
			providerEpisodes := []ProviderEpisode{
				{SeasonNumber: 0, EpisodeNumber: 1}, {SeasonNumber: 1, EpisodeNumber: 1},
				{SeasonNumber: 2, EpisodeNumber: 1},
			}
			if anime {
				svc.episodeProvider = &fakeEpisodeProvider{episodes: providerEpisodes, resolveID: 42}
			}
			svc.httpClient = &http.Client{Transport: airingFixtureTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"name":"Show","external_ids":{"tvdb_id":42},"seasons":[{"season_number":0},{"season_number":1},{"season_number":2}]}`
				if strings.Contains(r.URL.Path, "/season/") {
					body = `{"episodes":[{"episode_number":1},{"episode_number":2}]}`
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			seriesType := "standard"
			if anime {
				seriesType = "anime"
			}
			show, err := svc.AddSeries(ctx, &AddSeriesRequest{TMDBID: "123", SeriesType: seriesType})
			if err != nil {
				t.Fatal(err)
			}
			for _, season := range repo.seasons[show.ID] {
				if season.Monitored != (season.SeasonNumber != 0) {
					t.Fatalf("unexpected default monitoring: %+v", season)
				}
			}
			if _, err := svc.SetSeasonMonitoring(ctx, show.ID, 0, true); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.SetSeasonMonitoring(ctx, show.ID, 2, false); err != nil {
				t.Fatal(err)
			}
			// Preserve per-episode choices too, even within a monitored season.
			for _, ep := range repo.episodes[show.ID] {
				if strings.Contains(ep.SeasonID, "-s01") && ep.EpisodeNumber == 1 {
					ep.Monitored = false
				}
			}
			if anime {
				svc.episodeProvider = &fakeEpisodeProvider{resolveID: 42, episodes: append(providerEpisodes,
					ProviderEpisode{SeasonNumber: 2, EpisodeNumber: 2}, ProviderEpisode{SeasonNumber: 3, EpisodeNumber: 1})}
			}
			if err := svc.RefreshSeries(ctx, show.ID); err != nil {
				t.Fatal(err)
			}
			for _, season := range repo.seasons[show.ID] {
				if season.Monitored != (season.SeasonNumber != 2) {
					t.Fatalf("refresh reset season: %+v", season)
				}
			}
			for _, ep := range repo.episodes[show.ID] {
				disabledSeason := strings.Contains(ep.SeasonID, "-s02")
				disabledEpisode := strings.Contains(ep.SeasonID, "-s01") && ep.EpisodeNumber == 1
				want := !disabledSeason && !disabledEpisode
				if ep.Monitored != want {
					t.Fatalf("refresh reset episode: %+v, want monitored=%v", ep, want)
				}
			}
		})
	}
}
