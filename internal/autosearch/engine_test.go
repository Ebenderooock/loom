package autosearch

import (
	"io"
	"log/slog"
	"net/url"
	"testing"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

func intPtr(v int) *int { return &v }

func TestMatchQualityDef(t *testing.T) {
	// Use the same Source/Resolution/Modifier/Name values as seed.go
	// to verify the matching works with real data.
	defs := []*movies.QualityDefinition{
		{ID: "hdtv-1080", Name: "hdtv-1080p", Source: "TV", Resolution: "1080p"},
		{ID: "webdl-1080", Name: "webdl-1080p", Source: "Web", Resolution: "1080p"},
		{ID: "webrip-1080", Name: "webrip-1080p", Source: "WebRip", Resolution: "1080p"},
		{ID: "bluray-1080", Name: "bluray-1080p", Source: "BluRay", Resolution: "1080p"},
		{ID: "bluray-1080-remux", Name: "bluray-1080p-remux", Source: "BluRay", Resolution: "1080p", Modifier: "REMUX"},
		{ID: "hdtv-720", Name: "hdtv-720p", Source: "TV", Resolution: "720p"},
		{ID: "webdl-720", Name: "webdl-720p", Source: "Web", Resolution: "720p"},
		{ID: "bluray-2160", Name: "bluray-2160p", Source: "BluRay", Resolution: "2160p"},
		{ID: "bluray-2160-remux", Name: "bluray-2160p-remux", Source: "BluRay", Resolution: "2160p", Modifier: "REMUX"},
		{ID: "webdl-2160", Name: "webdl-2160p", Source: "Web", Resolution: "2160p"},
	}

	tests := []struct {
		name    string
		release *parser.Release
		wantID  string
		wantNil bool
	}{
		{
			name:    "bluray 1080p",
			release: &parser.Release{Source: "BluRay", Resolution: 1080},
			wantID:  "bluray-1080",
		},
		{
			name:    "webdl 1080p (parser returns WEB-DL)",
			release: &parser.Release{Source: "WEB-DL", Resolution: 1080},
			wantID:  "webdl-1080",
		},
		{
			name:    "webdl 1080p (parser returns WebDL)",
			release: &parser.Release{Source: "WebDL", Resolution: 1080},
			wantID:  "webdl-1080",
		},
		{
			name:    "hdtv 1080p",
			release: &parser.Release{Source: "HDTV", Resolution: 1080},
			wantID:  "hdtv-1080",
		},
		{
			name:    "webrip 1080p",
			release: &parser.Release{Source: "WEBRip", Resolution: 1080},
			wantID:  "webrip-1080",
		},
		{
			name:    "bluray 1080p remux",
			release: &parser.Release{Source: "BluRay", Resolution: 1080, IsRemux: true},
			wantID:  "bluray-1080-remux",
		},
		{
			name:    "bluray 2160p remux",
			release: &parser.Release{Source: "BluRay", Resolution: 2160, IsRemux: true},
			wantID:  "bluray-2160-remux",
		},
		{
			name:    "bluray 2160p non-remux",
			release: &parser.Release{Source: "BluRay", Resolution: 2160},
			wantID:  "bluray-2160",
		},
		{
			name:    "webdl 2160p",
			release: &parser.Release{Source: "WEB-DL", Resolution: 2160},
			wantID:  "webdl-2160",
		},
		{
			name:    "resolution-only fallback (unknown source, 720p)",
			release: &parser.Release{Source: "", Resolution: 720},
			wantID:  "hdtv-720",
		},
		{
			name:    "no match (480p not in definitions)",
			release: &parser.Release{Source: "DVDRip", Resolution: 480},
			wantNil: true,
		},
		{
			name:    "nil release",
			release: nil,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchQualityDef(tt.release, defs)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil quality def")
			}
			if got.ID != tt.wantID {
				t.Errorf("got ID %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestNormalizeSource(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"BluRay", "bluray"},
		{"BDRip", "bluray"},
		{"WEB-DL", "webdl"},
		{"WebDL", "webdl"},
		{"WebRip", "webrip"},
		{"HDTV", "hdtv"},
		{"DVDRip", "dvd"},
		{"Remux", "remux"},
		{"CAM", "cam"},
		{"TS", "cam"},
		{"Screener", "screener"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeSource(tt.input)
			if got != tt.want {
				t.Errorf("normalizeSource(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeDefSource(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Web", "webdl"},
		{"TV", "hdtv"},
		{"BluRay", "bluray"},
		{"WebRip", "webrip"},
		{"DVD", "dvd"},
		{"Unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeDefSource(tt.input)
			if got != tt.want {
				t.Errorf("normalizeDefSource(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestComputeTiebreaker(t *testing.T) {
	t.Run("torrent with seeders", func(t *testing.T) {
		score := computeTiebreaker(indexers.Result{
			Seeders:   intPtr(50),
			Size:      5 * 1024 * 1024 * 1024, // 5GB
			Freeleech: true,
		})
		if score <= 0 {
			t.Errorf("expected positive score, got %f", score)
		}
		if score > 100 {
			t.Errorf("expected score <= 100, got %f", score)
		}
	})

	t.Run("zero seeder torrent", func(t *testing.T) {
		score := computeTiebreaker(indexers.Result{
			Seeders: intPtr(0),
			Size:    1024 * 1024 * 1024,
		})
		// Zero seeders should still produce some score from size
		if score < 0 {
			t.Errorf("expected non-negative score, got %f", score)
		}
	})

	t.Run("usenet result (nil seeders)", func(t *testing.T) {
		score := computeTiebreaker(indexers.Result{
			Seeders: nil,
			Size:    2 * 1024 * 1024 * 1024,
		})
		if score < 20 {
			t.Errorf("expected usenet base score >= 20, got %f", score)
		}
	})

	t.Run("freeleech bonus", func(t *testing.T) {
		base := computeTiebreaker(indexers.Result{
			Seeders: intPtr(10),
			Size:    1024 * 1024 * 1024,
		})
		withFl := computeTiebreaker(indexers.Result{
			Seeders:   intPtr(10),
			Size:      1024 * 1024 * 1024,
			Freeleech: true,
		})
		diff := withFl - base
		if diff < 14 || diff > 16 {
			t.Errorf("expected ~15 freeleech bonus, got %f", diff)
		}
	})
}

func TestScoredReleaseCompositeScore(t *testing.T) {
	t.Run("quality tier dominates", func(t *testing.T) {
		better := ScoredRelease{QualityTier: 0, FormatScore: 0, TiebreakerScore: 0}
		worse := ScoredRelease{QualityTier: 5, FormatScore: 100, TiebreakerScore: 100}
		if better.CompositeScore() <= worse.CompositeScore() {
			t.Errorf("better quality tier (%f) should beat worse (%f)",
				better.CompositeScore(), worse.CompositeScore())
		}
	})

	t.Run("format score breaks quality tie", func(t *testing.T) {
		a := ScoredRelease{QualityTier: 3, FormatScore: 500, TiebreakerScore: 0}
		b := ScoredRelease{QualityTier: 3, FormatScore: 100, TiebreakerScore: 100}
		if a.CompositeScore() <= b.CompositeScore() {
			t.Errorf("higher format score (%f) should beat lower (%f)",
				a.CompositeScore(), b.CompositeScore())
		}
	})

	t.Run("tiebreaker breaks everything-else tie", func(t *testing.T) {
		a := ScoredRelease{QualityTier: 3, FormatScore: 100, TiebreakerScore: 80}
		b := ScoredRelease{QualityTier: 3, FormatScore: 100, TiebreakerScore: 20}
		if a.CompositeScore() <= b.CompositeScore() {
			t.Errorf("higher tiebreaker (%f) should beat lower (%f)",
				a.CompositeScore(), b.CompositeScore())
		}
	})
}

func TestBuildReleaseInfo(t *testing.T) {
	rel := &parser.Release{
		Source:     "BluRay",
		Resolution: 1080,
		Codec:      "h265",
		Group:      "SPARKS",
	}
	res := &indexers.Result{
		Title:     "Movie.2024.1080p.BluRay.x265-SPARKS",
		IndexerID: "idx-1",
		Size:      5000000000,
		Freeleech: true,
		Internal:  false,
		Scene:     true,
	}

	ri := buildReleaseInfo(rel, res)

	if ri.Title != res.Title {
		t.Errorf("title: got %q, want %q", ri.Title, res.Title)
	}
	if ri.Source != "BluRay" {
		t.Errorf("source: got %q, want %q", ri.Source, "BluRay")
	}
	if ri.Resolution != "1080p" {
		t.Errorf("resolution: got %q, want %q", ri.Resolution, "1080p")
	}
	if ri.Codec != "h265" {
		t.Errorf("codec: got %q, want %q", ri.Codec, "h265")
	}
	if ri.Group != "SPARKS" {
		t.Errorf("group: got %q, want %q", ri.Group, "SPARKS")
	}
	if ri.Indexer != "idx-1" {
		t.Errorf("indexer: got %q, want %q", ri.Indexer, "idx-1")
	}
	if len(ri.IndexerFlags) != 2 {
		t.Fatalf("expected 2 flags, got %d: %v", len(ri.IndexerFlags), ri.IndexerFlags)
	}
}

func TestInferProtocol(t *testing.T) {
	tests := []struct {
		name string
		res  indexers.Result
		want string
	}{
		{"magnet", indexers.Result{MagnetURI: "magnet:?xt=..."}, "torrent"},
		{"infohash", indexers.Result{Infohash: "abc123"}, "torrent"},
		{"seeders", indexers.Result{Seeders: intPtr(5)}, "torrent"},
		{"nzb link", indexers.Result{Link: "https://nzb.example.com/dl.nzb"}, "usenet"},
		{"nzb in url", indexers.Result{Link: "https://example.com/nzbget/file"}, "usenet"},
		{"plain link", indexers.Result{Link: "https://example.com/download"}, "torrent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferProtocol(&tt.res)
			if string(got) != tt.want {
				t.Errorf("inferProtocol() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildDownloadRequest_InfohashUsesTrackerRichMagnet(t *testing.T) {
	req := buildDownloadRequest(&indexers.Result{
		Title:    "Off Campus S01E02 The Practice 1080p AMZN WEB-DL DDP5 1 H 264-FLUX",
		Infohash: "4ADB30F0BCB29FD663F60E7E4EBC38A3611C4CB3",
	})
	if req.Magnet == "" {
		t.Fatal("expected magnet built from infohash")
	}
	u, err := url.Parse(req.Magnet)
	if err != nil {
		t.Fatalf("parse magnet: %v", err)
	}
	q := u.Query()
	if got := q.Get("dn"); got != "Off Campus S01E02 The Practice 1080p AMZN WEB-DL DDP5 1 H 264-FLUX" {
		t.Fatalf("dn = %q", got)
	}
	if got := len(q["tr"]); got < 10 {
		t.Fatalf("tracker count = %d, want >= 10", got)
	}
}

func TestEvaluateResult_QualityFiltering(t *testing.T) {
	defs := []*movies.QualityDefinition{
		{ID: "bluray-1080", Name: "Bluray-1080p", Source: "bluray", Resolution: "1080p"},
		{ID: "webdl-720", Name: "WEBDL-720p", Source: "webdl", Resolution: "720p"},
	}

	allowedMap := map[string]int{"bluray-1080": 0}
	allowedDefs := map[string]bool{"bluray-1080": true}
	formatScores := map[string]int{}

	cfEngine := customformats.NewEngine(nil)

	engine := &Engine{cfEngine: cfEngine}

	t.Run("allowed quality passes", func(t *testing.T) {
		res := indexers.Result{
			Title:   "Movie.2024.1080p.BluRay.x265-GROUP",
			Seeders: intPtr(10),
			Size:    2 * 1024 * 1024 * 1024,
		}
		sr := engine.evaluateResult(SearchRequest{}, res, defs, allowedMap, allowedDefs, formatScores, &qualityprofiles.QualityProfile{}, existingQuality{}, -1, false)
		if sr.Rejected {
			t.Errorf("expected not rejected, got rejected: %s", sr.RejectReason)
		}
		if sr.QualityDef == nil || sr.QualityDef.ID != "bluray-1080" {
			t.Errorf("expected quality def bluray-1080, got %v", sr.QualityDef)
		}
	})

	t.Run("disallowed quality rejected", func(t *testing.T) {
		res := indexers.Result{
			Title:   "Movie.2024.720p.WEB-DL.x264-GROUP",
			Seeders: intPtr(10),
			Size:    1024 * 1024 * 1024,
		}
		sr := engine.evaluateResult(SearchRequest{}, res, defs, allowedMap, allowedDefs, formatScores, &qualityprofiles.QualityProfile{}, existingQuality{}, -1, false)
		if !sr.Rejected {
			t.Error("expected rejected for disallowed quality")
		}
		if sr.RejectReason != "quality_not_allowed" {
			t.Errorf("expected reason quality_not_allowed, got %q", sr.RejectReason)
		}
	})

	t.Run("zero seeders rejected", func(t *testing.T) {
		res := indexers.Result{
			Title:   "Movie.2024.1080p.BluRay.x265-GROUP",
			Seeders: intPtr(0),
			Size:    2 * 1024 * 1024 * 1024,
		}
		sr := engine.evaluateResult(SearchRequest{}, res, defs, allowedMap, allowedDefs, formatScores, &qualityprofiles.QualityProfile{}, existingQuality{}, -1, false)
		if !sr.Rejected {
			t.Error("expected rejected for zero seeders")
		}
		if sr.RejectReason != "zero_seeders" {
			t.Errorf("expected reason zero_seeders, got %q", sr.RejectReason)
		}
	})
}

func TestEvaluateResult_SizeFiltering(t *testing.T) {
	defs := []*movies.QualityDefinition{
		{
			ID: "bluray-1080", Name: "Bluray-1080p", Source: "bluray", Resolution: "1080p",
			MinFileSize: 1 * 1024 * 1024 * 1024,  // 1GB
			MaxFileSize: 50 * 1024 * 1024 * 1024, // 50GB
		},
	}

	allowedMap := map[string]int{"bluray-1080": 0}
	allowedDefs := map[string]bool{"bluray-1080": true}
	formatScores := map[string]int{}

	cfEngine := customformats.NewEngine(nil)
	engine := &Engine{cfEngine: cfEngine}

	t.Run("below min size rejected", func(t *testing.T) {
		res := indexers.Result{
			Title:   "Movie.2024.1080p.BluRay.x265-GROUP",
			Seeders: intPtr(10),
			Size:    500 * 1024 * 1024, // 500MB
		}
		sr := engine.evaluateResult(SearchRequest{}, res, defs, allowedMap, allowedDefs, formatScores, &qualityprofiles.QualityProfile{}, existingQuality{}, -1, false)
		if !sr.Rejected {
			t.Error("expected rejected for below min size")
		}
		if sr.RejectReason != "below_min_size" {
			t.Errorf("expected reason below_min_size, got %q", sr.RejectReason)
		}
	})

	t.Run("above max size rejected", func(t *testing.T) {
		res := indexers.Result{
			Title:   "Movie.2024.1080p.BluRay.x265-GROUP",
			Seeders: intPtr(10),
			Size:    60 * 1024 * 1024 * 1024, // 60GB
		}
		sr := engine.evaluateResult(SearchRequest{}, res, defs, allowedMap, allowedDefs, formatScores, &qualityprofiles.QualityProfile{}, existingQuality{}, -1, false)
		if !sr.Rejected {
			t.Error("expected rejected for above max size")
		}
		if sr.RejectReason != "above_max_size" {
			t.Errorf("expected reason above_max_size, got %q", sr.RejectReason)
		}
	})
}

func TestEvaluateResult_MinFormatScore(t *testing.T) {
	defs := []*movies.QualityDefinition{
		{ID: "bluray-1080", Name: "Bluray-1080p", Source: "bluray", Resolution: "1080p"},
	}

	allowedMap := map[string]int{"bluray-1080": 0}
	allowedDefs := map[string]bool{"bluray-1080": true}
	formatScores := map[string]int{} // no format matches → total score = 0

	cfEngine := customformats.NewEngine(nil)
	engine := &Engine{cfEngine: cfEngine}

	t.Run("below min format score rejected", func(t *testing.T) {
		res := indexers.Result{
			Title:   "Movie.2024.1080p.BluRay.x265-GROUP",
			Seeders: intPtr(10),
			Size:    2 * 1024 * 1024 * 1024,
		}
		profile := &qualityprofiles.QualityProfile{MinFormatScore: 100}
		sr := engine.evaluateResult(SearchRequest{}, res, defs, allowedMap, allowedDefs, formatScores, profile, existingQuality{}, -1, false)
		if !sr.Rejected {
			t.Error("expected rejected for below min format score")
		}
		if sr.RejectReason != "below_min_format_score" {
			t.Errorf("expected reason below_min_format_score, got %q", sr.RejectReason)
		}
	})

	t.Run("zero min format score passes", func(t *testing.T) {
		res := indexers.Result{
			Title:   "Movie.2024.1080p.BluRay.x265-GROUP",
			Seeders: intPtr(10),
			Size:    2 * 1024 * 1024 * 1024,
		}
		profile := &qualityprofiles.QualityProfile{MinFormatScore: 0}
		sr := engine.evaluateResult(SearchRequest{}, res, defs, allowedMap, allowedDefs, formatScores, profile, existingQuality{}, -1, false)
		if sr.Rejected {
			t.Errorf("expected not rejected, got: %s", sr.RejectReason)
		}
	})
}

// ── verifyIdentity tests ──────────────────────────────────────────────────────

func newEngine() *Engine {
	return &Engine{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestVerifyIdentity_NilParsed(t *testing.T) {
	e := newEngine()
	if got := e.verifyIdentity(SearchRequest{}, nil, false); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestVerifyIdentity_TitleMatch(t *testing.T) {
	e := newEngine()
	tests := []struct {
		name         string
		reqTitle     string
		parsedTitle  string
		idBased      bool
		wantRejected bool
		wantReason   string
	}{
		{
			name:         "exact match passes",
			reqTitle:     "FROM",
			parsedTitle:  "FROM",
			wantRejected: false,
		},
		{
			name:         "case insensitive match passes",
			reqTitle:     "Breaking Bad",
			parsedTitle:  "breaking bad",
			wantRejected: false,
		},
		{
			name:         "mismatch rejected",
			reqTitle:     "FROM",
			parsedTitle:  "Fringe",
			wantRejected: true,
			wantReason:   "title_mismatch",
		},
		{
			name:         "id-based skips title check entirely",
			reqTitle:     "FROM",
			parsedTitle:  "Fringe",
			idBased:      true,
			wantRejected: false,
		},
		{
			name:         "empty parsed title skips check",
			reqTitle:     "FROM",
			parsedTitle:  "",
			wantRejected: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := SearchRequest{MediaType: "episode", Title: tc.reqTitle}
			parsed := &parser.Release{Title: tc.parsedTitle}
			got := e.verifyIdentity(req, parsed, tc.idBased)
			if tc.wantRejected && got != tc.wantReason {
				t.Errorf("expected reason %q, got %q", tc.wantReason, got)
			}
			if !tc.wantRejected && got != "" {
				t.Errorf("expected pass, got %q", got)
			}
		})
	}
}

func TestVerifyIdentity_SeasonPackTitle(t *testing.T) {
	// Regression: "FROM.S04.Complete.1080p" must parse title as "FROM",
	// not "FROM S04 Complete" — otherwise the season pack gets rejected
	// with title_mismatch. Verify via the parser + verifyIdentity pipeline.
	e := newEngine()
	parsed := parser.Parse("FROM.S04.Complete.1080p.WEB-DL.x264")
	if parsed == nil {
		t.Fatal("parse returned nil")
	}
	if parser.CleanSeriesTitle(parsed.Title) != "from" {
		t.Errorf("parser produced wrong title %q; want 'from'. Season pack title extraction broken.", parsed.Title)
	}
	req := SearchRequest{MediaType: "series", Title: "FROM", Season: 4}
	if got := e.verifyIdentity(req, parsed, false); got != "" {
		t.Errorf("season pack should not be rejected, got %q", got)
	}
}

func TestVerifyIdentity_EpisodeChecks(t *testing.T) {
	e := newEngine()
	tests := []struct {
		name       string
		req        SearchRequest
		parsed     *parser.Release
		wantReason string
	}{
		{
			name:   "correct episode passes",
			req:    SearchRequest{MediaType: "episode", Title: "FROM", Season: 4, Episode: 1},
			parsed: &parser.Release{Title: "FROM", Season: 4, Episode: 1},
		},
		{
			name:       "wrong episode rejected",
			req:        SearchRequest{MediaType: "episode", Title: "FROM", Season: 4, Episode: 1},
			parsed:     &parser.Release{Title: "FROM", Season: 4, Episode: 4},
			wantReason: "wrong_episode",
		},
		{
			name:       "wrong season rejected",
			req:        SearchRequest{MediaType: "episode", Title: "FROM", Season: 4, Episode: 1},
			parsed:     &parser.Release{Title: "FROM", Season: 3, Episode: 1},
			wantReason: "wrong_season",
		},
		{
			name:   "season pack for season search passes",
			req:    SearchRequest{MediaType: "series", Title: "FROM", Season: 4},
			parsed: &parser.Release{Title: "FROM", Season: 4, IsSeasonPack: true},
		},
		{
			name:       "single episode rejected for season search",
			req:        SearchRequest{MediaType: "series", Title: "FROM", Season: 4},
			parsed:     &parser.Release{Title: "FROM", Season: 4, Episode: 1},
			wantReason: "not_a_season_pack",
		},
		{
			name:       "season pack rejected for episode search",
			req:        SearchRequest{MediaType: "episode", Title: "FROM", Season: 4, Episode: 1},
			parsed:     &parser.Release{Title: "FROM", Season: 4, IsSeasonPack: true},
			wantReason: "season_pack_for_episode_search",
		},
		{
			name:   "multi-episode file contains requested episode — passes",
			req:    SearchRequest{MediaType: "episode", Title: "FROM", Season: 4, Episode: 1},
			parsed: &parser.Release{Title: "FROM", Season: 4, Episode: 1, Episodes: []int{1, 2, 3}},
		},
		{
			// Bug 3 regression: old code used `parsed.Episode > 0` which skipped
			// validation entirely when episode=0 (specials), allowing ep0 to
			// match any episode search. Fixed to `!= -1` so ep0 is properly checked.
			name:       "episode 0 (special) rejected for non-special search — Bug 3 regression",
			req:        SearchRequest{MediaType: "episode", Title: "FROM", Season: 1, Episode: 1},
			parsed:     &parser.Release{Title: "FROM", Season: 1, Episode: 0, Episodes: []int{}},
			wantReason: "wrong_episode",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := e.verifyIdentity(tc.req, tc.parsed, false)
			if got != tc.wantReason {
				t.Errorf("want reason %q, got %q", tc.wantReason, got)
			}
		})
	}
}

func TestVerifyIdentity_MovieYear(t *testing.T) {
	e := newEngine()
	tests := []struct {
		name       string
		reqYear    int
		parsedYear int
		wantReason string
	}{
		{"exact year passes", 2022, 2022, ""},
		{"off by 1 passes", 2022, 2023, ""},
		{"off by 2 rejected", 2022, 2024, "wrong_year"},
		{"no parsed year skips check", 2022, 0, ""},
		{"no req year skips check", 0, 2022, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := SearchRequest{MediaType: "movie", Title: "Inception", Year: tc.reqYear}
			parsed := &parser.Release{Title: "Inception", Year: tc.parsedYear}
			got := e.verifyIdentity(req, parsed, false)
			if got != tc.wantReason {
				t.Errorf("want %q, got %q", tc.wantReason, got)
			}
		})
	}
}

// TestVerifyIdentity_MovieYearlessRelease covers the case where parsed.Year==0
// because the year doesn't appear in a standard position, but the raw release
// name contains a year that differs from the requested year (Bug #102).
func TestVerifyIdentity_MovieYearlessRelease(t *testing.T) {
	e := newEngine()
	tests := []struct {
		name       string
		releaseName string
		reqYear    int
		wantReason string
	}{
		{
			name:        "old year in raw name rejected",
			releaseName: "Masters.of.the.Universe.1987.1080p.BluRay",
			reqYear:     2026,
			wantReason:  "wrong_year",
		},
		{
			name:        "matching year in raw name passes",
			releaseName: "Masters.of.the.Universe.2026.1080p.BluRay",
			reqYear:     2026,
			wantReason:  "",
		},
		{
			name:        "off-by-1 year in raw name passes",
			releaseName: "Masters.of.the.Universe.2025.1080p.BluRay",
			reqYear:     2026,
			wantReason:  "",
		},
		{
			name:        "no year in raw name passes (cannot discriminate)",
			releaseName: "Masters.of.the.Universe.1080p.BluRay",
			reqYear:     2026,
			wantReason:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := SearchRequest{MediaType: "movie", Title: "Masters of the Universe", Year: tc.reqYear}
			// Simulate a release where the parser extracted Title but Year==0
			// (year in unusual position or parser missed it).
			parsed := &parser.Release{
				Name:  tc.releaseName,
				Title: "Masters of the Universe",
				Year:  0,
			}
			got := e.verifyIdentity(req, parsed, false)
			if got != tc.wantReason {
				t.Errorf("want %q, got %q", tc.wantReason, got)
			}
		})
	}
}

// ── buildQueryChain tests ─────────────────────────────────────────────────────

func TestBuildQueryChain_EpisodeWithIDs(t *testing.T) {
	e := newEngine()
	req := SearchRequest{
		MediaType: "episode",
		Title:     "FROM",
		Season:    4, Episode: 1,
		TVDBID: "12345",
		TMDBID: "99999",
	}
	tiers := e.buildQueryChain(req)
	if len(tiers) != 2 {
		t.Fatalf("expected 2 tiers (ID + title), got %d", len(tiers))
	}
	// Tier 0: ID-based
	if tiers[0][0].TVDBID != "12345" {
		t.Errorf("tier 0 should carry TVDB ID")
	}
	if tiers[0][0].Mode != indexers.ModeTVSearch {
		t.Errorf("tier 0 should be tvsearch mode")
	}
	// Tier 1: title-based
	if tiers[1][0].Term != "FROM" {
		t.Errorf("tier 1 term should be FROM, got %q", tiers[1][0].Term)
	}
}

func TestBuildQueryChain_EpisodeNoIDs(t *testing.T) {
	e := newEngine()
	req := SearchRequest{MediaType: "episode", Title: "FROM", Season: 4, Episode: 1}
	tiers := e.buildQueryChain(req)
	if len(tiers) != 1 {
		t.Fatalf("expected 1 tier (title only), got %d", len(tiers))
	}
	if tiers[0][0].Term != "FROM" {
		t.Errorf("expected term FROM, got %q", tiers[0][0].Term)
	}
	if tiers[0][0].Season != 4 || tiers[0][0].Episode != 1 {
		t.Errorf("season/episode not propagated")
	}
}

func TestBuildQueryChain_MovieWithIMDB(t *testing.T) {
	e := newEngine()
	req := SearchRequest{
		MediaType: "movie",
		Title:     "Inception",
		Year:      2010,
		IMDBID:    "tt1375666",
		TMDBID:    "27205",
	}
	tiers := e.buildQueryChain(req)
	if len(tiers) != 2 {
		t.Fatalf("expected 2 tiers, got %d", len(tiers))
	}
	// Tier 0 movie ID query
	if tiers[0][0].Mode != indexers.ModeMovie {
		t.Errorf("movie tier 0 should be movie mode")
	}
	if tiers[0][0].IMDBID != "tt1375666" {
		t.Errorf("IMDB ID not set in tier 0")
	}
	// Tier 1 title query
	if tiers[1][0].Mode != indexers.ModeSearch {
		t.Errorf("movie title tier should be search mode")
	}
	if tiers[1][0].Year != 2010 {
		t.Errorf("year not propagated in title tier")
	}
}

func TestBuildQueryChain_AlternateTitles(t *testing.T) {
	e := newEngine()
	req := SearchRequest{
		MediaType:       "episode",
		Title:           "FROM",
		AlternateTitles: []string{"FROM (2022)", ""},
	}
	tiers := e.buildQueryChain(req)
	// Should have 1 tier with 2 queries (primary + one non-empty alt)
	if len(tiers) != 1 {
		t.Fatalf("expected 1 tier, got %d", len(tiers))
	}
	if len(tiers[0]) != 2 {
		t.Errorf("expected 2 title queries, got %d", len(tiers[0]))
	}
}

func TestBuildQueryChain_TVCategories(t *testing.T) {
	e := newEngine()
	tiers := e.buildQueryChain(SearchRequest{MediaType: "episode", Title: "FROM"})
	for _, q := range tiers[0] {
		found := false
		for _, cat := range q.Categories {
			if cat == 5000 {
				found = true
				break
			}
		}
		if !found {
			t.Error("TV categories (5000 range) not set for episode search")
		}
	}
}

func TestBuildQueryChain_MovieCategories(t *testing.T) {
	e := newEngine()
	tiers := e.buildQueryChain(SearchRequest{MediaType: "movie", Title: "Inception"})
	for _, q := range tiers[0] {
		found := false
		for _, cat := range q.Categories {
			if cat == 2000 {
				found = true
				break
			}
		}
		if !found {
			t.Error("movie categories (2000 range) not set for movie search")
		}
	}
}
