package music

import (
	"encoding/json"
	"testing"
)

func testDefs() []*AudioQualityDefinition {
	return []*AudioQualityDefinition{
		{ID: "aq_unknown", Name: "Unknown", TierOrder: 0},
		{ID: "aq_mp3_128", Name: "MP3-128", Format: "MP3", Bitrate: 128, TierOrder: 1},
		{ID: "aq_mp3_v2", Name: "MP3-V2", Format: "MP3", VBR: true, TierOrder: 2},
		{ID: "aq_mp3_256", Name: "MP3-256", Format: "MP3", Bitrate: 256, TierOrder: 3},
		{ID: "aq_mp3_v0", Name: "MP3-V0", Format: "MP3", VBR: true, TierOrder: 4},
		{ID: "aq_mp3_320", Name: "MP3-320", Format: "MP3", Bitrate: 320, TierOrder: 5},
		{ID: "aq_aac_256", Name: "AAC-256", Format: "AAC", Bitrate: 256, TierOrder: 6},
		{ID: "aq_aac_320", Name: "AAC-320", Format: "AAC", Bitrate: 320, TierOrder: 7},
		{ID: "aq_flac", Name: "FLAC", Format: "FLAC", Lossless: true, TierOrder: 8},
		{ID: "aq_flac_24", Name: "FLAC 24bit", Format: "FLAC", Lossless: true, TierOrder: 9},
	}
}

func TestMatchAudioQuality(t *testing.T) {
	defs := testDefs()
	tests := []struct {
		title string
		want  string
	}{
		{"Artist - Album [FLAC]", "aq_flac"},
		{"Artist - Album [24bit FLAC]", "aq_flac_24"},
		{"Artist - Album [MP3 320]", "aq_mp3_320"},
		{"Artist - Album [MP3 256]", "aq_mp3_256"},
		{"Artist - Album [MP3 128]", "aq_mp3_128"},
		{"Artist - Album MP3-V0", "aq_mp3_v0"},
		{"Artist - Album MP3-V2", "aq_mp3_v2"},
		{"Artist - Album [AAC 320]", "aq_aac_320"},
		{"Artist - Album [AAC 256]", "aq_aac_256"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			rel := ParseMusic(tt.title)
			got := MatchAudioQuality(rel, defs)
			if got == nil {
				t.Fatalf("MatchAudioQuality returned nil, want %s", tt.want)
			}
			if got.ID != tt.want {
				t.Errorf("MatchAudioQuality = %s, want %s", got.ID, tt.want)
			}
		})
	}
}

func TestMatchAudioQualityEmptyDefs(t *testing.T) {
	if got := MatchAudioQuality(ParseMusic("x - y [FLAC]"), nil); got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

func TestScoreAudioReleaseNilProfile(t *testing.T) {
	defs := testDefs()
	score := ScoreAudioRelease(ParseMusic("Artist - Album [FLAC]"), defs, nil, 10, 1000)
	if !score.Allowed {
		t.Error("nil profile should accept all")
	}
	if !score.MeetsCutoff {
		t.Error("nil profile should meet cutoff")
	}
	if score.Tier != 8 {
		t.Errorf("Tier = %d, want 8", score.Tier)
	}
}

func TestScoreAudioReleaseProfileGating(t *testing.T) {
	defs := testDefs()
	items, _ := json.Marshal([]AudioProfileItem{
		{DefinitionID: "aq_mp3_320", Allowed: true},
		{DefinitionID: "aq_flac", Allowed: true},
		{DefinitionID: "aq_flac_24", Allowed: true},
	})
	profile := &AudioQualityProfile{ID: "p", Items: items, Cutoff: "aq_flac"}

	allowed := ScoreAudioRelease(ParseMusic("Artist - Album [FLAC]"), defs, profile, 5, 0)
	if !allowed.Allowed {
		t.Error("FLAC should be allowed")
	}
	if !allowed.MeetsCutoff {
		t.Error("FLAC should meet aq_flac cutoff")
	}

	denied := ScoreAudioRelease(ParseMusic("Artist - Album [MP3 128]"), defs, profile, 5, 0)
	if denied.Allowed {
		t.Error("MP3-128 should be denied by profile")
	}

	below := ScoreAudioRelease(ParseMusic("Artist - Album [MP3 320]"), defs, profile, 5, 0)
	if !below.Allowed {
		t.Error("MP3-320 should be allowed")
	}
	if below.MeetsCutoff {
		t.Error("MP3-320 is below aq_flac cutoff")
	}
}

func TestCompositeOrdersByTier(t *testing.T) {
	defs := testDefs()
	flac := ScoreAudioRelease(ParseMusic("Artist - Album [FLAC]"), defs, nil, 1, 0)
	mp3 := ScoreAudioRelease(ParseMusic("Artist - Album [MP3 320]"), defs, nil, 1000, 0)
	if flac.Composite() <= mp3.Composite() {
		t.Errorf("FLAC composite %.2f should exceed high-seed MP3 %.2f", flac.Composite(), mp3.Composite())
	}
}

func TestTiebreakerBySeeders(t *testing.T) {
	defs := testDefs()
	a := ScoreAudioRelease(ParseMusic("Artist - Album [FLAC]"), defs, nil, 100, 0)
	b := ScoreAudioRelease(ParseMusic("Artist - Album [FLAC]"), defs, nil, 5, 0)
	if a.Composite() <= b.Composite() {
		t.Errorf("higher seeders should win tiebreak: %.4f vs %.4f", a.Composite(), b.Composite())
	}
}
