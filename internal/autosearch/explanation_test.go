package autosearch

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

func explanationEngine() (*Engine, []*movies.QualityDefinition) {
	e := newEngine()
	e.cfEngine = customformats.NewEngine([]customformats.CustomFormat{{
		ID: "group", Name: "Preferred group",
		Specifications: []customformats.Specification{{
			Implementation: customformats.ImplReleaseTitle, Fields: map[string]any{"value": "PREFERRED"},
		}},
	}})
	return e, []*movies.QualityDefinition{
		{ID: "bluray", Name: "bluray-1080p", Title: "BluRay 1080p", Source: "BluRay", Resolution: "1080p"},
		{ID: "web", Name: "webdl-1080p", Title: "WEB 1080p", Source: "Web", Resolution: "1080p"},
	}
}

func TestExplanationFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/explanations.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name     string
		Request  SearchRequest
		Title    string
		Reason   string
		Category string
		Absolute int
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	e, defs := explanationEngine()
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			sr := e.evaluateResult(f.Request, indexers.Result{Title: f.Title}, defs,
				map[string]int{"bluray": 0}, map[string]bool{"bluray": true}, nil,
				&qualityprofiles.QualityProfile{}, existingQuality{}, -1, f.Request.IMDBID != "" || f.Request.TMDBID != "")
			if sr.RejectReason != f.Reason || sr.Explanation.Category != f.Category {
				t.Fatalf("got %q / %q, want %q / %q (parsed %+v)", sr.RejectReason, sr.Explanation.Category, f.Reason, f.Category, sr.Parsed)
			}
			if f.Absolute > 0 && sr.Explanation.Numbering.AbsoluteEpisode != f.Absolute {
				t.Fatalf("absolute number lost: %+v", sr.Explanation.Numbering)
			}
			if sr.Explanation.Summary == "" || len(sr.Explanation.Identity) == 0 {
				t.Fatal("missing actionable evidence")
			}
			if !reflect.DeepEqual(scoredToEval(sr).Explanation, sr.Explanation) {
				t.Fatal("search log must retain actual evaluation evidence")
			}
		})
	}
}

type explanationMovieService struct {
	movies.Service
	defs []*movies.QualityDefinition
}

func (s explanationMovieService) ListQualityDefinitions(context.Context) ([]*movies.QualityDefinition, error) {
	return s.defs, nil
}

func TestEvaluatePreviewParityAndRanking(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE quality_profiles_v2 (
		id TEXT PRIMARY KEY, name TEXT, cutoff TEXT, min_format_score INTEGER,
		cutoff_format_score INTEGER, upgrade_allowed INTEGER, items TEXT,
		created_at DATETIME, updated_at DATETIME);
		CREATE TABLE quality_profile_format_items (profile_id TEXT, format_id TEXT, score INTEGER);`)
	if err != nil {
		t.Fatal(err)
	}
	store := qualityprofiles.NewStore(db)
	profile := &qualityprofiles.QualityProfile{
		ID: "policy", Name: "Test", Items: `[{"id":"bluray","allowed":true},{"id":"web","allowed":true}]`,
		FormatItems: []qualityprofiles.FormatItem{{FormatID: "group", Score: 50000}},
	}
	if err := store.Create(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	e, defs := explanationEngine()
	e.profileStore = store
	e.movieSvc = explanationMovieService{defs: defs}
	now := time.Now()
	req := EvaluateRequest{SearchRequest: SearchRequest{Title: "Movie", QualityProfileID: profile.ID}, Results: []indexers.Result{
		{Title: "Movie.2024.1080p.WEB-DL-PREFERRED", PubDate: now},
		{Title: "Movie.2024.1080p.BluRay", PubDate: now},
		{Title: "Movie.2024.1080p.BluRay-PREFERRED", PubDate: now},
	}}
	live, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if live.Results[0].QualityTier != 0 || live.Results[0].FormatScore != 50000 ||
		live.Results[2].QualityTier != 1 || live.Results[2].CompositeScore <= live.Results[1].CompositeScore {
		t.Fatalf("must rank by lexicographic keys, never composite: %+v", live.Results)
	}
	if !strings.Contains(live.Results[0].Explanation.RankingReason, "custom-format") ||
		!strings.Contains(live.Results[2].Explanation.RankingReason, "quality tier") {
		t.Fatal("ranking must explain first differing production key")
	}
	if live.Results[0].Explanation.Contributions[0].Score != 50000 || live.Results[0].FormatMatches[0].Score != 50000 {
		t.Fatal("format contribution must be profile-specific")
	}
	req.PolicyPreview = profile
	preview, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Preview || len(live.Results) != len(preview.Results) {
		t.Fatal("preview result shape differs")
	}
	for i := range live.Results {
		a, b := live.Results[i], preview.Results[i]
		if math.Abs(a.CompositeScore-b.CompositeScore) > 0.01 {
			t.Fatal("preview scoring differs beyond elapsed age")
		}
		a.CompositeScore, b.CompositeScore = 0, 0
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("unchanged preview diverges from actual evaluation: %+v / %+v", a, b)
		}
	}
	draft := *profile
	draft.MinFormatScore = 50001
	req.PolicyPreview = &draft
	preview, err = e.Evaluate(context.Background(), req)
	if err != nil || preview.Passed != 0 {
		t.Fatalf("draft minimum not applied: %+v / %v", preview, err)
	}
	stored, err := store.Get(context.Background(), profile.ID)
	if err != nil || stored.MinFormatScore != 0 {
		t.Fatal("preview must never persist policy")
	}
}

func TestSizeEvidenceUsesEffectiveLimits(t *testing.T) {
	e, defs := explanationEngine()
	defs[0].SizeMode = movies.SizeModePerMinute
	defs[0].MinFileSize = 2
	sr := e.evaluateResult(SearchRequest{Runtime: 30}, indexers.Result{Title: "Movie.1080p.BluRay", Size: 60*1024*1024 - 1},
		defs, map[string]int{"bluray": 0}, map[string]bool{"bluray": true}, nil,
		&qualityprofiles.QualityProfile{}, existingQuality{}, -1, false)
	if sr.RejectReason != "below_min_size" {
		t.Fatal(sr.RejectReason)
	}
	for _, c := range sr.Explanation.Checks {
		if c.Field == "minimum size" {
			if c.Expected != ">= 62914560 (0 = disabled)" || c.Actual != "62914559" || c.Passed || !strings.HasPrefix(c.Unit, "bytes") {
				t.Fatalf("wrong byte threshold: %+v", c)
			}
			return
		}
	}
	t.Fatal("missing size comparison")
}

func TestInvalidPreviewHTTP(t *testing.T) {
	h := NewHandler(newEngine(), nil)
	for _, items := range []string{"null", "{}", `[{"id":"a"},{"id":"a"}]`} {
		body, err := json.Marshal(EvaluateRequest{
			PolicyPreview: &qualityprofiles.QualityProfile{Items: items},
		})
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.HandleEvaluate(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body))))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid preview returned %d", w.Code)
		}
	}
}
