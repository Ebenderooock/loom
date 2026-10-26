package calendar_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/ebenderooock/loom/internal/calendar"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
	"github.com/ebenderooock/loom/internal/workflows"
)

func TestCalendarMediaStatuses(t *testing.T) {
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
	store, err := workflows.NewStore(db.DB())
	if err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.DB().ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO series(id,title) VALUES ('show','Show');
		INSERT INTO seasons(id,series_id,season_number) VALUES ('season','show',1)`)
	want := map[string]string{}
	for _, tc := range []struct{ id, status, workflow, expected string }{
		{"missing", "missing", "", "missing"},
		{"unreleased", "unreleased", "", "unreleased"},
		{"downloading", "downloading", "", "downloading"},
		{"storing", "storing", "", "importing"},
		{"available", "available_right_quality", "downloading", "downloaded"},
		{"wrong-quality", "available_wrong_quality", "", "downloaded"},
		{"higher-quality", "available_higher_quality", "", "downloaded"},
		{"grabbed", "missing", "grabbed", "grabbed"},
		{"searching", "missing", "searching", "searching"},
		{"workflow-download", "missing", "downloading", "downloading"},
		{"post-download", "missing", "post_download", "importing"},
		{"workflow-import", "missing", "importing", "importing"},
		{"cleanup", "missing", "cleaning_up", "importing"},
		{"failed", "missing", "failed", "missing"},
		{"cancelled", "missing", "cancelled", "missing"},
		{"completed", "missing", "completed", "missing"},
	} {
		exec(`INSERT INTO movies(id,title,release_date,theatrical_date,digital_date,status,monitoring_status)
			VALUES (?,?,'2026-10-01','2026-10-01','2026-10-02',?,'monitored')`, tc.id, tc.id, tc.status)
		exec(`INSERT INTO episodes(id,series_id,season_id,episode_number,title,air_date,has_file,monitored)
			VALUES (?,'show','season',?,?, '2000-01-01',?,0)`, "ep-"+tc.id, len(want)+1, tc.id, tc.status == "available_right_quality")
		want[tc.id] = tc.expected
		if tc.workflow != "" {
			if err := store.Create(ctx, &workflows.Workflow{
				Type: workflows.TypeMovieSearch, State: tc.workflow, MediaType: "movie",
				Items: []workflows.Item{{MediaType: "movie", MediaID: tc.id}, {MediaType: "episode", MediaID: "ep-" + tc.id}},
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	router := calendar.Router(db.DB(), store)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/?start=2000-01-01&end=2026-10-31", nil))
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var events []calendar.Event
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != len(want)*4 {
		t.Fatalf("got %d events, want %d", len(events), len(want)*4)
	}
	for _, event := range events {
		if event.Type == "movie" {
			if event.Status != want[event.ID] || !event.Monitored {
				t.Errorf("movie status or monitoring: %+v, want %s", event, want[event.ID])
			}
		} else {
			if event.SeriesID != "show" || event.Monitored {
				t.Errorf("episode identity or monitoring: %+v", event)
			}
			switch event.ID {
			case "ep-grabbed", "ep-searching", "ep-workflow-download", "ep-post-download", "ep-workflow-import", "ep-cleanup":
				if event.Status != want[event.ID[3:]] {
					t.Errorf("episode workflow status: %+v", event)
				}
			case "ep-available":
				if event.Status != "downloaded" {
					t.Errorf("downloaded precedence: %+v", event)
				}
			default:
				if event.Status != "missing" {
					t.Errorf("episode should be missing: %+v", event)
				}
			}
		}
	}
}
