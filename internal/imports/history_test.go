package imports

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTitleHistoryFiltersBeforePaginationAndScopesSeriesEpisodes(t *testing.T) {
	db := setupTestDB(t)
	if _, err := db.Exec(`CREATE TABLE episodes(id TEXT PRIMARY KEY, series_id TEXT);
		INSERT INTO episodes VALUES('ep-a','show-a'),('ep-b','show-b')`); err != nil {
		t.Fatal(err)
	}
	for i, entry := range []struct{ id, mediaType, mediaID string }{
		{"movie-a-old", "movie", "movie-a"},
		{"show-a", "episode", "ep-a"},
		{"movie-b", "movie", "movie-b"},
		{"show-b", "episode", "ep-b"},
		{"movie-a-new", "movie", "movie-a"},
	} {
		if _, err := db.Exec(`INSERT INTO import_history
			(id,media_type,media_id,source_path,dest_path,import_mode,status,error,imported_at)
			VALUES(?,?,?,'source','dest','move','imported','',?)`,
			entry.id, entry.mediaType, entry.mediaID, time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
	}
	p := &ImportPipeline{db: db}
	for _, tc := range []struct {
		mediaType, mediaID string
		offset             int
		want               string
	}{
		{"movie", "movie-a", 0, "movie-a-new"},
		{"movie", "movie-a", 1, "movie-a-old"},
		{"series", "show-a", 0, "show-a"},
		{"series", "show-b", 0, "show-b"},
	} {
		records, err := p.ListTitleHistory(context.Background(), tc.mediaType, tc.mediaID, 1, tc.offset)
		if err != nil || len(records) != 1 || records[0].ID != tc.want {
			t.Fatalf("%+v: records=%+v err=%v", tc, records, err)
		}
		if tc.mediaType == "series" && (records[0].SeriesID != tc.mediaID || records[0].MediaType != MediaTypeEpisode) {
			t.Fatalf("lost original episode identity/series scope: %+v", records[0])
		}
	}
	handler := handleListHistory(p)
	for _, query := range []string{"media_id=movie-a", "media_type=series", "media_type=episode&media_id=ep-a"} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("GET", "/history?"+query, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid filter should not expose unrelated history: %s %d", query, w.Code)
		}
	}
	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest("GET", "/history?media_type=series&media_id=missing", nil))
	var body struct {
		Data []*ImportRecord `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 || body.Data == nil || len(body.Data) != 0 {
		t.Fatalf("empty scoped history shape: %d %s %v", w.Code, w.Body.String(), err)
	}
}
