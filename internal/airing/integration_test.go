package airing_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/calendar"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/scheduler"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/storage"
)

func libraryDB(t *testing.T) storage.DB {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: ":memory:"},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO series(id,title) VALUES ('show','Test Show');
		INSERT INTO seasons(id,series_id,season_number) VALUES ('season','show',1), ('specials','show',0);`); err != nil {
		t.Fatal(err)
	}
	return db
}

func putEpisode(t *testing.T, repo series.Repository, id, date, stamp string) *series.Episode {
	t.Helper()
	ep := &series.Episode{
		ID: id, SeriesID: "show", SeasonID: "season",
		Title: id, AirDate: date, AirDateUTC: stamp, Monitored: true,
		CreatedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	// Distinct episode numbers.
	episodes, err := repo.ListEpisodes(context.Background(), "show", nil)
	if err != nil {
		t.Fatal(err)
	}
	ep.EpisodeNumber = len(episodes) + 1
	if err := repo.CreateEpisode(context.Background(), ep); err != nil {
		t.Fatal(err)
	}
	return ep
}

func TestTimestampPersistenceStatsAndScheduler(t *testing.T) {
	ctx := context.Background()
	db := libraryDB(t)
	repo := series.NewRepository(db.DB())
	putEpisode(t, repo, "past", "2999-01-01", "2000-01-01T00:00:00Z")
	putEpisode(t, repo, "future", "2000-01-01", "2999-01-01T00:00:00Z")
	putEpisode(t, repo, "date-only", "2000-01-01", "")
	downloaded := putEpisode(t, repo, "downloaded", "2000-01-01", "2999-01-01T00:00:00Z")
	downloaded.HasFile = true
	unmonitored := putEpisode(t, repo, "unmonitored", "2000-01-01", "2000-01-01T00:00:00Z")
	unmonitored.Monitored = false
	special := putEpisode(t, repo, "special", "2000-01-01", "2000-01-01T00:00:00Z")
	special.SeasonID = "specials"
	for _, ep := range []*series.Episode{downloaded, unmonitored, special} {
		if err := repo.UpdateEpisode(ctx, ep); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.DB().Exec(`UPDATE episodes SET season_id = 'specials' WHERE id = 'special'`); err != nil {
		t.Fatal(err)
	}
	ep, err := repo.GetEpisode(ctx, "future")
	if err != nil || ep.AirDate != "2000-01-01" || ep.AirDateUTC != "2999-01-01T00:00:00Z" {
		t.Fatalf("round trip: %+v, %v", ep, err)
	}
	seasonNum := 1
	episodes, err := repo.ListEpisodes(ctx, "show", &seasonNum)
	if err != nil || len(episodes) != 5 {
		t.Fatalf("season query: %+v, %v", episodes, err)
	}
	stats, err := repo.GetEpisodeStats(ctx, "show")
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalEpisodes != 5 || stats.AiredEpisodes != 3 || stats.MissingEpisodes != 2 || stats.DownloadedEpisodes != 1 {
		t.Fatalf("stats: %+v", stats)
	}
	all, err := repo.GetAllEpisodeStats(ctx)
	if err != nil || *all["show"] != *stats {
		t.Fatalf("all stats: %+v, %v", all, err)
	}
	seasons, err := repo.GetSeasonEpisodeStats(ctx, "show")
	if err != nil || seasons["season"].AiredEpisodes != 3 || seasons["season"].MissingEpisodes != 2 || seasons["specials"].MissingEpisodes != 1 {
		t.Fatalf("season stats: %+v, %v", seasons, err)
	}
	store := scheduler.NewStore(db.DB())
	candidates, err := store.GetCandidates(ctx, "episode", 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool)
	for _, c := range candidates {
		found[c.MediaID] = true
	}
	if len(found) != 3 || !found["past"] || !found["date-only"] || !found["special"] {
		t.Fatalf("scheduler selected wrong episodes: %+v", candidates)
	}
	count, err := store.QueueSize(ctx, 1)
	if err != nil || count != len(candidates) {
		t.Fatalf("queue size=%d, err=%v", count, err)
	}
	// Enrichment must not undo a concurrent download or monitoring change.
	stale := *ep
	ep.HasFile, ep.Monitored = true, false
	if err := repo.UpdateEpisode(ctx, ep); err != nil {
		t.Fatal(err)
	}
	stale.AirDateUTC = "2999-01-01T01:00:00Z"
	if err := repo.UpdateEpisodeAiringTime(ctx, &stale); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetEpisode(ctx, ep.ID)
	if err != nil || !got.HasFile || got.Monitored || got.AirDateUTC != stale.AirDateUTC {
		t.Fatalf("airing update overwrote episode state: %+v, %v", got, err)
	}
}

func TestCalendarLocalRanges(t *testing.T) {
	db := libraryDB(t)
	repo := series.NewRepository(db.DB())
	putEpisode(t, repo, "nz-october-start", "2026-09-30", "2026-09-30T23:00:00Z")
	putEpisode(t, repo, "nz-november-start", "2026-10-31", "2026-11-01T00:00:00Z")
	putEpisode(t, repo, "us-october-end", "2026-11-01", "2026-11-01T02:00:00Z")
	putEpisode(t, repo, "date-only", "2026-10-21", "")
	putEpisode(t, repo, "us-evening", "2026-10-21", "2026-10-22T00:00:00Z")
	putEpisode(t, repo, "nz-dst-start", "2026-09-26", "2026-09-26T12:00:00Z")
	putEpisode(t, repo, "nz-dst-end", "2026-09-27", "2026-09-27T11:00:00Z")
	putEpisode(t, repo, "not-aired", "2000-01-01", "2999-01-01T00:00:00Z")
	downloaded := putEpisode(t, repo, "downloaded-early", "2000-01-01", "2999-01-01T00:00:00Z")
	downloaded.HasFile = true
	if err := repo.UpdateEpisode(context.Background(), downloaded); err != nil {
		t.Fatal(err)
	}
	router := calendar.Router(db.DB())
	for _, tc := range []struct {
		name, zone, start, end string
		want                   map[string]string
	}{
		{"NZ October", "Pacific/Auckland", "2026-10-01", "2026-10-31", map[string]string{
			"nz-october-start": "2026-10-01", "date-only": "2026-10-21", "us-evening": "2026-10-22",
		}},
		{"US October", "America/New_York", "2026-10-01", "2026-10-31", map[string]string{
			"nz-november-start": "2026-10-31", "us-october-end": "2026-10-31",
			"date-only": "2026-10-21", "us-evening": "2026-10-21",
		}},
		{"NZ 23 hour DST day", "Pacific/Auckland", "2026-09-27", "2026-09-27", map[string]string{
			"nz-dst-start": "2026-09-27",
		}},
		{"legacy UTC", "", "2026-10-21", "2026-10-21", map[string]string{"date-only": "2026-10-21"}},
		{"future", "Pacific/Auckland", "2999-01-01", "2999-01-01", map[string]string{
			"not-aired": "2999-01-01", "downloaded-early": "2999-01-01",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := url.Values{"start": {tc.start}, "end": {tc.end}, "timezone": {tc.zone}}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/?"+q.Encode(), nil))
			if w.Code != 200 {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			var events []calendar.Event
			if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
				t.Fatal(err)
			}
			if len(events) != len(tc.want) {
				t.Fatalf("events = %+v, want %v", events, tc.want)
			}
			for _, event := range events {
				if event.Date != tc.want[event.ID] {
					t.Errorf("event %+v", event)
				}
				if event.ID == "not-aired" && event.Status != "unaired" {
					t.Errorf("future episode status = %s", event.Status)
				}
				if event.ID == "downloaded-early" && event.Status != "downloaded" {
					t.Error("downloaded precedence lost")
				}
				if event.ID == "us-evening" && event.AirDateUTC != "2026-10-22T00:00:00Z" {
					t.Error("actual timestamp absent from calendar API")
				}
			}
		})
	}
	for _, query := range []string{"timezone=invalid", "start=bad", "start=2026-10-22&end=2026-10-21"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/?"+query, nil))
		if w.Code != 400 {
			t.Fatalf("query %q: status %d", query, w.Code)
		}
	}
}
