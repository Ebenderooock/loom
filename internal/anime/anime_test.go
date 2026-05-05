package anime

import "testing"

func TestParseAbsolute(t *testing.T) {
	r := Parse("[SubsPlease] Jujutsu Kaisen - 01 [1080p]")
	if r.ReleaseGroup != "SubsPlease" {
		t.Errorf("group = %q, want SubsPlease", r.ReleaseGroup)
	}
	if r.AbsoluteEpisode != 1 {
		t.Errorf("absolute = %d, want 1", r.AbsoluteEpisode)
	}
	if r.Title != "Jujutsu Kaisen" {
		t.Errorf("title = %q, want Jujutsu Kaisen", r.Title)
	}
	if r.Resolution != 1080 {
		t.Errorf("resolution = %d, want 1080", r.Resolution)
	}
}

func TestParseSxEWithAbsolute(t *testing.T) {
	r := Parse("Naruto Shippuden - S01E01 - 001 [1080p] [Dual-Audio]")
	if r.Season != 1 {
		t.Errorf("season = %d, want 1", r.Season)
	}
	if r.Episode != 1 {
		t.Errorf("episode = %d, want 1", r.Episode)
	}
	if r.AbsoluteEpisode != 1 {
		t.Errorf("absolute = %d, want 1", r.AbsoluteEpisode)
	}
	if !r.IsDualAudio {
		t.Error("expected dual audio")
	}
}

func TestParseDualAudio(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"[EMBER] Title - 01 Dual-Audio [1080p]", true},
		{"[EMBER] Title - 01 Dual Audio [1080p]", true},
		{"[EMBER] Title - 01 DUAL [1080p]", true},
		{"[SubsPlease] Title - 01 [1080p]", false},
	}
	for _, tt := range tests {
		r := Parse(tt.name)
		if r.IsDualAudio != tt.want {
			t.Errorf("Parse(%q).IsDualAudio = %v, want %v", tt.name, r.IsDualAudio, tt.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	r := Parse("[SubsPlease] Title - 05v2 [1080p]")
	if r.Version != 2 {
		t.Errorf("version = %d, want 2", r.Version)
	}
}

func TestParseBatch(t *testing.T) {
	r := Parse("[Kametsu] Title [Batch] [1080p] [BluRay]")
	if !r.IsBatch {
		t.Error("expected batch")
	}
	if r.Source != "BluRay" {
		t.Errorf("source = %q, want BluRay", r.Source)
	}
}

func TestParseSubType(t *testing.T) {
	r := Parse("[Group] Title - 01 [hardsub] [720p]")
	if r.SubType != SubTypeHardsub {
		t.Errorf("subType = %q, want hardsub", r.SubType)
	}
}

func TestParseERAINotRaw(t *testing.T) {
	// "Erai-raws" contains "raw" but should not flag as raw subtype
	r := Parse("[Erai-raws] Title - 01 [1080p]")
	if r.SubType == SubTypeRaw {
		t.Error("Erai-raws should not be detected as raw")
	}
	if r.ReleaseGroup != "Erai-raws" {
		t.Errorf("group = %q, want Erai-raws", r.ReleaseGroup)
	}
}

func TestParseCodec(t *testing.T) {
	r := Parse("[EMBER] Title - 01 [1080p] [HEVC]")
	if r.Codec != "h265" {
		t.Errorf("codec = %q, want h265", r.Codec)
	}
}

func TestMapAbsoluteToSeason(t *testing.T) {
	mappings := GenerateOffsetMappings(2, 26, 25) // abs 26-50 = S02E01-25
	s, e := MapAbsoluteToSeason(26, mappings)
	if s != 2 || e != 1 {
		t.Errorf("got S%02dE%02d, want S02E01", s, e)
	}
	s, e = MapAbsoluteToSeason(50, mappings)
	if s != 2 || e != 25 {
		t.Errorf("got S%02dE%02d, want S02E25", s, e)
	}
}

func TestMapSeasonToAbsolute(t *testing.T) {
	mappings := GenerateOffsetMappings(2, 26, 25)
	abs := MapSeasonToAbsolute(2, 1, mappings)
	if abs != 26 {
		t.Errorf("got %d, want 26", abs)
	}
}

func TestScoreGroup(t *testing.T) {
	score := ScoreGroup("EMBER", nil)
	if score != Tier1Score {
		t.Errorf("EMBER score = %d, want %d", score, Tier1Score)
	}

	score = ScoreGroup("SubsPlease", nil)
	if score != Tier2Score {
		t.Errorf("SubsPlease score = %d, want %d", score, Tier2Score)
	}

	// Custom override
	score = ScoreGroup("MyGroup", map[string]int{"MyGroup": 42})
	if score != 42 {
		t.Errorf("custom score = %d, want 42", score)
	}

	// Unknown group
	score = ScoreGroup("Unknown", nil)
	if score != 0 {
		t.Errorf("unknown score = %d, want 0", score)
	}
}

func TestValidateMappings(t *testing.T) {
	err := ValidateMappings([]EpisodeMapping{
		{AbsoluteNumber: 1, SeasonNumber: 1, EpisodeNumber: 1},
		{AbsoluteNumber: 2, SeasonNumber: 1, EpisodeNumber: 2},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Duplicate
	err = ValidateMappings([]EpisodeMapping{
		{AbsoluteNumber: 1, SeasonNumber: 1, EpisodeNumber: 1},
		{AbsoluteNumber: 1, SeasonNumber: 1, EpisodeNumber: 2},
	})
	if err == nil {
		t.Error("expected error for duplicate absolute number")
	}

	// Invalid absolute
	err = ValidateMappings([]EpisodeMapping{
		{AbsoluteNumber: 0, SeasonNumber: 1, EpisodeNumber: 1},
	})
	if err == nil {
		t.Error("expected error for zero absolute number")
	}
}
