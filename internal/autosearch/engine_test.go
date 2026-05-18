package autosearch

import (
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
		name       string
		release    *parser.Release
		wantID     string
		wantNil    bool
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
		Codec:     "h265",
		Group:     "SPARKS",
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
			MaxFileSize: 50 * 1024 * 1024 * 1024,  // 50GB
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
