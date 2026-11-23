package customformats

import (
	"testing"

	"github.com/ebenderooock/loom/internal/parser"
)

func TestParseReleaseName(t *testing.T) {
	ri := ParseReleaseName("Movie.Name.2024.2160p.BluRay.x265.DTS-HD.MA-FraMeSToR")
	if ri.Resolution != "2160p" {
		t.Errorf("resolution = %q, want 2160p", ri.Resolution)
	}
	if ri.Source != "BluRay" {
		t.Errorf("source = %q, want BluRay", ri.Source)
	}
	if ri.Codec != "x265" {
		t.Errorf("codec = %q, want x265", ri.Codec)
	}
	if ri.Audio != "DTS-HD MA" {
		t.Errorf("audio = %q, want DTS-HD MA", ri.Audio)
	}
	if ri.Group != "FraMeSToR" {
		t.Errorf("group = %q, want FraMeSToR", ri.Group)
	}
}

func TestTesterAndAcquisitionUseSameBodyMetadata(t *testing.T) {
	title := "the.body.2026.s01e07.1080p.web.h264-cakes[EZTVx.to].mkv"
	tested := ParseReleaseName(title)
	acquired := FromParsedRelease(parser.Parse(title))
	if tested.Source != "WEB-DL" || tested.Codec != "x264" || tested.Quality != "webdl-1080p" {
		t.Fatalf("unexpected Body metadata: %+v", tested)
	}
	format := CustomFormat{ID: "body", Specifications: []Specification{
		{Implementation: ImplSource, Fields: map[string]any{"value": "WebDL"}},
		{Implementation: ImplCodec, Fields: map[string]any{"value": "h264"}},
		{Implementation: ImplQuality, Fields: map[string]any{"value": "webdl-1080p"}},
	}}
	engine := NewEngine([]CustomFormat{format})
	for _, release := range []ReleaseInfo{tested, acquired} {
		matches := engine.ScoreRelease(release)
		if len(matches) != 1 || ApplyScores(matches, map[string]int{"body": 125}) != 125 {
			t.Fatalf("tester/acquisition rules must match and use profile points: %+v", release)
		}
	}
}

func TestFileInfoRulesAreEnforcedAfterDownload(t *testing.T) {
	format := CustomFormat{ID: "file", Specifications: []Specification{{
		Implementation: ImplFileInfo, Required: true,
		Fields: map[string]any{"codec": "x264", "resolution": "1080p", "min_size_gb": 1.0},
	}}}
	engine := NewEngine([]CustomFormat{format})
	release := ParseReleaseName("Show.S01E01.1080p.WEB.H264.mkv")
	if len(engine.ScoreRelease(release)) != 1 {
		t.Fatal("release-only evaluation should defer file conditions")
	}
	file := FileMetadata{Codec: "h264", Resolution: "1080p", Size: 2 * 1024 * 1024 * 1024}
	if len(engine.ScoreFileMetadata(release, file)) != 1 {
		t.Fatal("matching file properties should be accepted")
	}
	file.Size = 1024
	if len(engine.ScoreFileMetadata(release, file)) != 0 {
		t.Fatal("actual file size must enforce post-download format conditions")
	}
	format.Specifications[0].Negate = true
	if len(engine.ScoreFileMetadata(release, file)) != 1 {
		t.Fatal("file conditions must honor negation")
	}
}

func TestParseReleaseNameWebDL(t *testing.T) {
	ri := ParseReleaseName("Show.S01E01.1080p.WEB-DL.AAC.x264-GROUP")
	if ri.Resolution != "1080p" {
		t.Errorf("resolution = %q, want 1080p", ri.Resolution)
	}
	if ri.Source != "WEB-DL" {
		t.Errorf("source = %q, want WEB-DL", ri.Source)
	}
	if ri.Codec != "x264" {
		t.Errorf("codec = %q, want x264", ri.Codec)
	}
	if ri.Audio != "AAC" {
		t.Errorf("audio = %q, want AAC", ri.Audio)
	}
	if ri.Group != "GROUP" {
		t.Errorf("group = %q, want GROUP", ri.Group)
	}
}

func TestEngineScoreRelease(t *testing.T) {
	formats := []CustomFormat{
		{
			ID: "prefer-hevc", Name: "Prefer HEVC",
			Specifications: []Specification{
				{Name: "x265", Implementation: ImplCodec, Fields: map[string]any{"value": "x265"}},
			},
		},
		{
			ID: "prefer-bluray", Name: "Prefer BluRay",
			Specifications: []Specification{
				{Name: "BluRay", Implementation: ImplSource, Fields: map[string]any{"value": "BluRay"}},
			},
		},
		{
			ID: "no-match", Name: "Prefer AV1",
			Specifications: []Specification{
				{Name: "AV1", Implementation: ImplCodec, Fields: map[string]any{"value": "AV1"}},
			},
		},
	}

	engine := NewEngine(formats)
	ri := ParseReleaseName("Movie.2024.2160p.BluRay.x265.DTS-FraMeSToR")
	matches := engine.ScoreRelease(ri)

	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(matches))
	}
	// Scores now come from quality profiles, not from the format itself.
	total := TotalScore(matches)
	if total != 0 {
		t.Errorf("total score = %d, want 0 (scores assigned by quality profiles)", total)
	}
}

func TestNegateSpec(t *testing.T) {
	cf := CustomFormat{
		ID: "not-x264", Name: "Not x264",
		Specifications: []Specification{
			{Name: "Not x264", Implementation: ImplCodec, Negate: true, Fields: map[string]any{"value": "x264"}},
		},
	}
	engine := NewEngine([]CustomFormat{cf})

	// x265 release should match (negate: x264 doesn't match → true)
	ri := ParseReleaseName("Movie.2024.1080p.BluRay.x265-GROUP")
	matches := engine.ScoreRelease(ri)
	if len(matches) != 1 {
		t.Errorf("expected 1 match for x265 release, got %d", len(matches))
	}

	// x264 release should NOT match (negate: x264 matches → false)
	ri2 := ParseReleaseName("Movie.2024.1080p.BluRay.x264-GROUP")
	matches2 := engine.ScoreRelease(ri2)
	if len(matches2) != 0 {
		t.Errorf("expected 0 matches for x264 release, got %d", len(matches2))
	}
}

func TestRequiredSpecMustMatch(t *testing.T) {
	cf := CustomFormat{
		ID: "test", Name: "Test",
		Specifications: []Specification{
			{Name: "Must be BluRay", Implementation: ImplSource, Required: true, Fields: map[string]any{"value": "BluRay"}},
			{Name: "Prefer x265", Implementation: ImplCodec, Fields: map[string]any{"value": "x265"}},
		},
	}
	engine := NewEngine([]CustomFormat{cf})

	// BluRay + x265 → match
	ri := ParseReleaseName("Movie.2024.BluRay.x265-GROUP")
	if len(engine.ScoreRelease(ri)) != 1 {
		t.Error("expected match for BluRay+x265")
	}

	// WEB-DL + x265 → no match (required BluRay not met)
	ri2 := ParseReleaseName("Movie.2024.WEB-DL.x265-GROUP")
	if len(engine.ScoreRelease(ri2)) != 0 {
		t.Error("expected no match for WEB-DL+x265")
	}
}
