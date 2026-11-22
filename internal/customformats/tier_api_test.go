package customformats

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

func TestTierAPIProfileAndTester(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`CREATE TABLE custom_quality_tiers (id TEXT PRIMARY KEY, name TEXT, specifications TEXT, created_at TEXT, updated_at TEXT);
		CREATE TABLE custom_formats (id TEXT PRIMARY KEY, name TEXT, include_when_renaming INTEGER, specifications TEXT, created_at TEXT, updated_at TEXT);
		CREATE TABLE quality_profiles_v2 (id TEXT PRIMARY KEY, name TEXT, cutoff TEXT, min_format_score INTEGER, cutoff_format_score INTEGER, upgrade_allowed INTEGER, items TEXT, created_at DATETIME, updated_at DATETIME);
		CREATE TABLE quality_profile_format_items (profile_id TEXT, format_id TEXT, score INTEGER);
		CREATE TABLE quality_profiles (id TEXT, name TEXT, cutoff TEXT, upgrade_allowed INTEGER, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME);
		CREATE TABLE movie_files (quality TEXT); CREATE TABLE episode_files (quality TEXT);`)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := NewStore(db)
	handler := Router(store, logger)
	tier := testTier("", "HEVC")
	body, _ := json.Marshal(tier)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/tiers", bytes.NewReader(body)))
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &tier); err != nil || tier.ID == "" {
		t.Fatalf("create identity: %+v %v", tier, err)
	}
	items, _ := json.Marshal([]qualityprofiles.QualityItem{{ID: tier.ID, Name: tier.Name, Kind: CustomKind, Allowed: true}})
	profile := &qualityprofiles.QualityProfile{ID: "p", Name: "P", Items: string(items), MinFormatScore: 1}
	if err := qualityprofiles.NewStore(db).Create(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(
		`{"title":"The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv","quality_profile_id":"p"}`)))
	var tested struct {
		Tier    *Resolution `json:"quality_tier"`
		Meets   bool        `json:"meets_minimum"`
		Release ReleaseInfo `json:"release"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &tested) != nil || tested.Tier == nil || tested.Tier.ID != tier.ID || tested.Meets || tested.Release.Source != "" {
		t.Fatalf("tester: %d %s", response.Code, response.Body)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/tiers/"+tier.ID, nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("referenced tier deletion: %d %s", response.Code, response.Body)
	}
	tier.Name = "Renamed"
	body, _ = json.Marshal(tier)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/tiers/"+tier.ID, bytes.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", response.Code, response.Body)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(
		`{"title":"The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv","quality_profile_id":"p"}`)))
	if err := json.Unmarshal(response.Body.Bytes(), &tested); err != nil || tested.Tier == nil || tested.Tier.Name != "Renamed" || tested.Tier.ID != tier.ID {
		t.Fatalf("rename tester: %s (%v)", response.Body, err)
	}
	if err := qualityprofiles.NewStore(db).Delete(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO episode_files (quality) VALUES (?)`, StoredTierID(tier.ID)); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/tiers/"+tier.ID, nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("file reference must prevent deletion: %d %s", response.Code, response.Body)
	}
	if _, err := db.Exec(`DELETE FROM episode_files`); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/tiers/"+tier.ID, nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("unreferenced delete: %d %s", response.Code, response.Body)
	}
	if err := qualityprofiles.NewStore(db).Create(context.Background(), profile); err == nil {
		t.Fatal("profile accepted deleted tier")
	}
}
