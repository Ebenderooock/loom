package qualityprofiles

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestProfileSavesRollbackOnFormatFailure(t *testing.T) {
	for _, operation := range []string{"create", "update", "set-scores"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			store := newSeedTestStore(t)
			original := &QualityProfile{
				ID: "profile", Name: "Original", MinFormatScore: 25,
				FormatItems: []FormatItem{{FormatID: "kept", Score: -15}},
			}
			if err := store.Create(ctx, original); err != nil {
				t.Fatal(err)
			}
			before, err := store.Get(ctx, original.ID)
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.db.Exec(`CREATE TRIGGER reject_format BEFORE INSERT ON quality_profile_format_items
				WHEN NEW.format_id = 'blocked' BEGIN SELECT RAISE(FAIL, 'blocked format'); END`)
			if err != nil {
				t.Fatal(err)
			}
			changed := *before
			changed.Name = "Changed"
			changed.MinFormatScore = 100
			changed.FormatItems = []FormatItem{{FormatID: "new", Score: 100}, {FormatID: "blocked", Score: 20}}
			switch operation {
			case "create":
				changed.ID = "new-profile"
				err = store.Create(ctx, &changed)
			case "update":
				err = store.Update(ctx, &changed)
			case "set-scores":
				err = store.SetFormatScores(ctx, original.ID, changed.FormatItems)
			}
			if err == nil {
				t.Fatal("expected format insert to fail")
			}
			after, err := store.Get(ctx, original.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("failed save changed the profile: before=%+v after=%+v", before, after)
			}
			var count int
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM quality_profiles_v2`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("failed create left a profile behind: count=%d", count)
			}
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM quality_profile_format_items`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("failed save left partial scores behind: count=%d", count)
			}
		})
	}
}

func TestProfileAPIRejectsInvalidFormatIDsWithoutChanges(t *testing.T) {
	for _, operation := range []string{"create", "update", "set-scores"} {
		for _, invalid := range []struct {
			name  string
			items string
		}{
			{"missing-id", `[{"custom_format_id":"preferred","score":100}]`},
			{"blank-id", `[{"format_id":" ","score":100}]`},
			{"duplicate-id", `[{"format_id":"preferred","score":100},{"format_id":"preferred","score":-25}]`},
		} {
			t.Run(operation+"/"+invalid.name, func(t *testing.T) {
				ctx := context.Background()
				store := newSeedTestStore(t)
				profile := &QualityProfile{ID: "profile", Name: "Original",
					FormatItems: []FormatItem{{FormatID: "kept", Score: 30}}}
				if err := store.Create(ctx, profile); err != nil {
					t.Fatal(err)
				}
				before, err := store.Get(ctx, profile.ID)
				if err != nil {
					t.Fatal(err)
				}
				method, path := http.MethodPut, "/profile"
				body := `{"name":"Changed","format_items":` + invalid.items + `}`
				switch operation {
				case "create":
					method, path = http.MethodPost, "/"
				case "set-scores":
					path, body = "/profile/format-scores", invalid.items
				}
				response := httptest.NewRecorder()
				Router(store, slog.New(slog.NewTextHandler(io.Discard, nil))).ServeHTTP(response,
					httptest.NewRequest(method, path, strings.NewReader(body)))
				if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "format_id") {
					t.Fatalf("expected actionable 400, got %d: %s", response.Code, response.Body.String())
				}
				after, err := store.Get(ctx, profile.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("invalid request changed the profile: before=%+v after=%+v", before, after)
				}
				var count int
				if err := store.db.QueryRow(`SELECT COUNT(*) FROM quality_profiles_v2`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Fatalf("invalid request created a profile: count=%d", count)
				}
			})
		}
	}
}

func TestProfileAPICreatesAndEditsMultipleFormatScores(t *testing.T) {
	store := newSeedTestStore(t)
	router := Router(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	profile := QualityProfile{ID: "profile", Name: "New profile", MinFormatScore: 50,
		FormatItems: []FormatItem{{FormatID: "preferred", Score: 100}, {FormatID: "avoid", Score: -25}}}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPut} {
		path, status := "/", http.StatusCreated
		if method == http.MethodPut {
			path, status = "/profile", http.StatusOK
			profile.Name = "Edited profile"
			profile.FormatItems[1].Score = -50
		}
		body, err := json.Marshal(profile)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, path, bytes.NewReader(body)))
		if response.Code != status {
			t.Fatalf("%s: got %d: %s", method, response.Code, response.Body.String())
		}
		stored, err := store.Get(context.Background(), profile.ID)
		if err != nil {
			t.Fatal(err)
		}
		scores := make(map[string]int)
		for _, item := range stored.FormatItems {
			scores[item.FormatID] = item.Score
		}
		if stored.Name != profile.Name || stored.MinFormatScore != 50 || len(scores) != 2 ||
			scores["preferred"] != 100 || scores["avoid"] != profile.FormatItems[1].Score {
			t.Fatalf("profile did not round-trip: %+v", stored)
		}
		if method == http.MethodPost {
			if _, err := store.db.Exec(`INSERT INTO quality_profile_format_items (profile_id, format_id, score)
				VALUES ('profile', '', 500)`); err != nil {
				t.Fatal(err)
			}
		}
	}
}
