package searchdebug

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openSearchDebugDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
CREATE TABLE search_debug_log (
	id TEXT PRIMARY KEY,
	created_at DATETIME NOT NULL,
	media_type TEXT NOT NULL DEFAULT '',
	media_id TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL DEFAULT '',
	year INTEGER NOT NULL DEFAULT 0,
	season INTEGER NOT NULL DEFAULT 0,
	episode INTEGER NOT NULL DEFAULT 0,
	imdb_id TEXT NOT NULL DEFAULT '',
	tvdb_id TEXT NOT NULL DEFAULT '',
	tmdb_id TEXT NOT NULL DEFAULT '',
	quality_profile_id TEXT NOT NULL DEFAULT '',
	request_json TEXT NOT NULL DEFAULT '{}',
	tiers_json TEXT NOT NULL DEFAULT '[]',
	indexer_results_json TEXT NOT NULL DEFAULT '[]',
	evaluation_json TEXT NOT NULL DEFAULT '[]',
	total_results INTEGER NOT NULL DEFAULT 0,
	total_rejected INTEGER NOT NULL DEFAULT 0,
	grabbed_title TEXT NOT NULL DEFAULT '',
	outcome TEXT NOT NULL DEFAULT '',
	duration_ms INTEGER NOT NULL DEFAULT 0,
	error_message TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'completed',
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	search_run_id TEXT NOT NULL DEFAULT ''
);`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return db
}

func seedEntry(t *testing.T, s *Store, id, title, outcome, status string, createdAt time.Time) {
	t.Helper()
	err := s.Create(context.Background(), &Entry{
		ID:        id,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		Status:    status,
		MediaType: "movie",
		MediaID:   "m1",
		Title:     title,
		Outcome:   outcome,
	})
	if err != nil {
		t.Fatalf("seed entry: %v", err)
	}
}

func TestSearchDebugHandlers_ListGetStatsAndClear(t *testing.T) {
	t.Parallel()
	db := openSearchDebugDB(t)
	store := NewStore(db)
	hub := NewHub()
	router := Router(store, hub, func(next http.Handler) http.Handler { return next })

	now := time.Now().UTC()
	seedEntry(t, store, "e1", "Movie One", "grabbed", StatusCompleted, now)
	seedEntry(t, store, "e2", "Movie Two", "all_rejected", StatusFailed, now.Add(-time.Minute))

	listReq := httptest.NewRequest(http.MethodGet, "/?limit=1&offset=0&media_type=movie", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listBody struct {
		Entries []Entry `json:"entries"`
		Total   int     `json:"total"`
		Limit   int     `json:"limit"`
		Offset  int     `json:"offset"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listBody.Total != 2 || len(listBody.Entries) != 1 || listBody.Limit != 1 {
		t.Fatalf("unexpected list body: %+v", listBody)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/e1", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", getRec.Code, getRec.Body.String())
	}
	var got Entry
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.ID != "e1" || got.Title != "Movie One" {
		t.Fatalf("unexpected get entry: %+v", got)
	}

	missReq := httptest.NewRequest(http.MethodGet, "/missing", nil)
	missRec := httptest.NewRecorder()
	router.ServeHTTP(missRec, missReq)
	if missRec.Code != http.StatusNotFound {
		t.Fatalf("missing get status = %d", missRec.Code)
	}

	statsReq := httptest.NewRequest(http.MethodGet, "/stats", nil)
	statsRec := httptest.NewRecorder()
	router.ServeHTTP(statsRec, statsReq)
	if statsRec.Code != http.StatusOK {
		t.Fatalf("stats status = %d", statsRec.Code)
	}
	var stats StatsResult
	if err := json.Unmarshal(statsRec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.TotalSearches < 2 {
		t.Fatalf("expected stats to include seeded rows: %+v", stats)
	}

	clearReq := httptest.NewRequest(http.MethodDelete, "/", nil)
	clearRec := httptest.NewRecorder()
	router.ServeHTTP(clearRec, clearReq)
	if clearRec.Code != http.StatusNoContent {
		t.Fatalf("clear status = %d", clearRec.Code)
	}

	afterClearReq := httptest.NewRequest(http.MethodGet, "/", nil)
	afterClearRec := httptest.NewRecorder()
	router.ServeHTTP(afterClearRec, afterClearReq)
	if afterClearRec.Code != http.StatusOK {
		t.Fatalf("list after clear status = %d", afterClearRec.Code)
	}
	var after struct {
		Entries []Entry `json:"entries"`
		Total   int     `json:"total"`
	}
	if err := json.Unmarshal(afterClearRec.Body.Bytes(), &after); err != nil {
		t.Fatalf("decode after clear list: %v", err)
	}
	if after.Total != 0 || len(after.Entries) != 0 {
		t.Fatalf("expected empty state after clear, got %+v", after)
	}
}
