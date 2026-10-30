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

func TestDiscoverySeriesAddDefaultsToAny(t *testing.T) {
	t.Parallel()
	for _, selection := range []string{"", "  ", "default", "selected-profile"} {
		t.Run("selection="+selection, func(t *testing.T) {
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
			if _, err := db.DB().ExecContext(ctx, `INSERT INTO quality_profiles_v2(id,name,cutoff) VALUES('any-profile','Any','480p'),('selected-profile','4K','2160p')`); err != nil {
				t.Fatal(err)
			}
			svc := NewService(NewRepository(db.DB()), "").(*service)
			svc.httpClient = &http.Client{Transport: airingFixtureTransport(func(_ *http.Request) (*http.Response, error) {
				body := `{"name":"Discovery Show","first_air_date":"2026-01-01","seasons":[]}`
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			request, err := json.Marshal(AddSeriesRequest{TMDBID: "123", LibraryID: "lib", QualityProfileID: selection})
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			Router(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(request))))
			if rec.Code != http.StatusCreated {
				t.Fatalf("add series status=%d body=%s", rec.Code, rec.Body)
			}
			var response struct {
				ID               string `json:"id"`
				QualityProfileID string `json:"qualityProfileId"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			want := selection
			if strings.TrimSpace(selection) == "" || selection == "default" {
				want = "any-profile"
			}
			got, err := svc.repo.GetSeries(ctx, response.ID)
			if err != nil || got.QualityProfileID != want || response.QualityProfileID != want {
				t.Fatalf("default not persisted/returned: got=%+v response=%+v err=%v", got, response, err)
			}
		})
	}
}
