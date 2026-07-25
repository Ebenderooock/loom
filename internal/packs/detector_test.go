package packs

import "testing"

func TestDetectPackPatterns(t *testing.T) {
	t.Parallel()
	tests := []struct {
		title       string
		wantPack    bool
		wantType    PackType
		seasonStart int
		seasonEnd   int
		epStart     int
		epEnd       int
	}{
		{"Show.S01-S03.1080p", true, PackTypeMultiSeason, 1, 3, 0, 0},
		{"Show.S02.Complete.720p", true, PackTypeSingleSeason, 2, 2, 0, 0},
		{"Show.Complete.Series.1080p", true, PackTypeCompleteSeries, -1, -1, 0, 0},
		{"Show.S01E01-E10.1080p", true, PackTypeEpisodeRange, 1, 1, 1, 10},
		{"Show.S04.720p", true, PackTypeSingleSeason, 4, 4, 0, 0},
		{"Show.S01E02.720p", false, "", 0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := Detect(tt.title)
			if got.IsPack != tt.wantPack {
				t.Fatalf("IsPack = %v, want %v", got.IsPack, tt.wantPack)
			}
			if !tt.wantPack {
				return
			}
			if got.Type != tt.wantType {
				t.Fatalf("Type = %s, want %s", got.Type, tt.wantType)
			}
			if got.SeasonStart != tt.seasonStart || got.SeasonEnd != tt.seasonEnd {
				t.Fatalf("season range = %d-%d, want %d-%d", got.SeasonStart, got.SeasonEnd, tt.seasonStart, tt.seasonEnd)
			}
			if got.EpisodeStart != tt.epStart || got.EpisodeEnd != tt.epEnd {
				t.Fatalf("episode range = %d-%d, want %d-%d", got.EpisodeStart, got.EpisodeEnd, tt.epStart, tt.epEnd)
			}
		})
	}
}

func TestIsPackAndCleanSeriesTitle(t *testing.T) {
	t.Parallel()
	if !IsPack("Series.Name.S01-S02.1080p") {
		t.Fatal("expected IsPack to detect multi-season pack")
	}
	if IsPack("Series.Name.S01E01.1080p") {
		t.Fatal("single episode should not be considered pack")
	}

	title := CleanSeriesTitle("Series.Name.S01-S02.1080p.BluRay")
	if title != "Series Name" {
		t.Fatalf("CleanSeriesTitle = %q, want %q", title, "Series Name")
	}
}
