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

func TestFormatTesterAppliesSelectedProfileScores(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE custom_formats (id TEXT PRIMARY KEY, name TEXT, include_when_renaming INTEGER,
			specifications TEXT, created_at TEXT, updated_at TEXT);
		CREATE TABLE quality_profiles_v2 (id TEXT PRIMARY KEY, name TEXT, cutoff TEXT,
			min_format_score INTEGER, cutoff_format_score INTEGER, upgrade_allowed INTEGER,
			items TEXT, created_at DATETIME, updated_at DATETIME);
		CREATE TABLE quality_profile_format_items (profile_id TEXT, format_id TEXT, score INTEGER,
			PRIMARY KEY (profile_id, format_id));
		CREATE TABLE quality_profiles (id TEXT, name TEXT, cutoff TEXT, upgrade_allowed INTEGER,
			created_at DATETIME, updated_at DATETIME, deleted_at DATETIME);`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store := NewStore(db)
	for _, format := range []CustomFormat{
		{ID: "web", Name: "WEB", Specifications: []Specification{{Implementation: ImplSource, Fields: map[string]any{"value": "WEB-DL"}}}},
		{ID: "penalty", Name: "Penalty", Specifications: []Specification{{Implementation: ImplCodec, Fields: map[string]any{"value": "x264"}}}},
		{ID: "unassigned", Name: "Not assigned", Specifications: []Specification{{Implementation: ImplResolution, Fields: map[string]any{"value": "1080p"}}}},
	} {
		if err := store.Create(ctx, &format); err != nil {
			t.Fatal(err)
		}
	}
	profile := qualityprofiles.QualityProfile{ID: "body", Name: "Body", MinFormatScore: 90,
		FormatItems: []qualityprofiles.FormatItem{{FormatID: "web", Score: 100}, {FormatID: "penalty", Score: -15}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	profileHandler := qualityprofiles.Router(qualityprofiles.NewStore(db), logger)
	profileBody, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	saved := httptest.NewRecorder()
	profileHandler.ServeHTTP(saved, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(profileBody)))
	if saved.Code != http.StatusCreated {
		t.Fatalf("create profile: status=%d body=%s", saved.Code, saved.Body)
	}
	handler := Router(store, logger)
	for _, profileID := range []string{"", "body", "missing"} {
		t.Run("profile="+profileID, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{
				"title":              "the.body.2026.s01e07.1080p.web.h264-cakes[EZTVx.to].mkv",
				"quality_profile_id": profileID,
			})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body)))
			if profileID == "missing" {
				if response.Code != http.StatusNotFound {
					t.Fatalf("missing profile: status=%d body=%s", response.Code, response.Body)
				}
				return
			}
			var result struct {
				Release          ReleaseInfo   `json:"release"`
				Matches          []FormatMatch `json:"matches"`
				Score            int           `json:"score"`
				QualityProfileID string        `json:"quality_profile_id"`
				MinimumScore     int           `json:"minimum_score"`
				MeetsMinimum     *bool         `json:"meets_minimum"`
			}
			if response.Code != http.StatusOK {
				t.Fatalf("test failed: status=%d body=%s", response.Code, response.Body)
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Matches) != 3 || result.Release.Quality != "webdl-1080p" {
				t.Fatalf("matching formats and base quality must be returned: %+v", result)
			}
			if profileID == "" {
				if result.Score != 0 || result.MeetsMinimum != nil {
					t.Fatalf("match-only test must not invent profile points: %+v", result)
				}
			} else if result.Score != 85 || result.MinimumScore != 90 || result.MeetsMinimum == nil || *result.MeetsMinimum || result.QualityProfileID != profileID {
				t.Fatalf("expected 100 - 15 = 85, below minimum 90: %+v", result)
			}
		})
	}

	t.Run("edited scores apply immediately", func(t *testing.T) {
		profile.MinFormatScore = 50
		profile.FormatItems[1].Score = -40
		body, err := json.Marshal(profile)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		profileHandler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/body", bytes.NewReader(body)))
		if response.Code != http.StatusOK {
			t.Fatalf("update profile: status=%d body=%s", response.Code, response.Body)
		}
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(
			`{"title":"the.body.2026.s01e07.1080p.web.h264-cakes[EZTVx.to].mkv","quality_profile_id":"body"}`)))
		var result struct {
			Score        int           `json:"score"`
			MinimumScore int           `json:"minimum_score"`
			MeetsMinimum bool          `json:"meets_minimum"`
			Matches      []FormatMatch `json:"matches"`
		}
		if response.Code != http.StatusOK {
			t.Fatalf("test profile: status=%d body=%s", response.Code, response.Body)
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Score != 60 || result.MinimumScore != 50 || !result.MeetsMinimum {
			t.Fatalf("updated scores must total 100 - 40 = 60 and meet minimum 50: %+v", result)
		}
		for _, match := range result.Matches {
			expected := map[string]int{"web": 100, "penalty": -40, "unassigned": 0}
			if match.Score != expected[match.CustomFormatID] {
				t.Fatalf("unexpected contribution: %+v", match)
			}
		}
	})
}
