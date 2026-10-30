package series

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

func TestNewlyAddedSeriesDetailsLoadFromPersistedRecord(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := storage.Open(ctx, config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "series.db")},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO quality_profiles_v2(id,name,cutoff) VALUES('any-profile','Any','480p')`); err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewRepository(db.DB()), "").(*service)
	svc.httpClient = &http.Client{Transport: airingFixtureTransport(func(_ *http.Request) (*http.Response, error) {
		body := `{"name":"Fresh Show","first_air_date":"2026-01-01","seasons":[{"season_number":1,"episode_count":1}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	router := Router(svc)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"tmdbId":"1399","libraryId":"shows","monitoringStatus":"all","seasonFolder":true,"seriesType":"standard"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("add status=%d body=%s", rec.Code, rec.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("created id=%q err=%v", created.ID, err)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+created.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing-series", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing series status=%d body=%s", rec.Code, rec.Body)
	}
}
