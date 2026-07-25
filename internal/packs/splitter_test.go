package packs

import "testing"

func TestDecidePack(t *testing.T) {
	t.Parallel()
	wanted := []WantedEpisodes{
		{Season: 1, EpisodeNums: []int{1, 2, 3, 4, 5}, TotalInSeason: 10},
	}

	tests := []struct {
		name       string
		pack       DetectedPack
		wantGrab   bool
		wantReason string
	}{
		{
			name:       "not a pack",
			pack:       DetectedPack{IsPack: false},
			wantGrab:   false,
			wantReason: "not a pack",
		},
		{
			name:       "50 percent threshold grabs",
			pack:       DetectedPack{IsPack: true, Type: PackTypeSingleSeason, SeasonStart: 1, SeasonEnd: 1},
			wantGrab:   true,
			wantReason: "≥50% of episodes wanted",
		},
		{
			name:       "below threshold prefers individual",
			pack:       DetectedPack{IsPack: true, Type: PackTypeEpisodeRange, SeasonStart: 1, SeasonEnd: 1, EpisodeStart: 1, EpisodeEnd: 10},
			wantGrab:   true,
			wantReason: "≥50% of episodes wanted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := DecidePack(tt.pack, wanted, 10_000_000_000, 1_000_000_000)
			if d.ShouldGrabPack != tt.wantGrab {
				t.Fatalf("ShouldGrabPack = %v, want %v", d.ShouldGrabPack, tt.wantGrab)
			}
			if d.Reason != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", d.Reason, tt.wantReason)
			}
		})
	}
}

func TestDecidePackBelowThresholdAndNoRange(t *testing.T) {
	t.Parallel()
	pack := DetectedPack{IsPack: true, Type: PackTypeEpisodeRange, SeasonStart: 1, SeasonEnd: 1, EpisodeStart: 1, EpisodeEnd: 10}
	wanted := []WantedEpisodes{{Season: 1, EpisodeNums: []int{1, 2, 3}, TotalInSeason: 10}}
	d := DecidePack(pack, wanted, 0, 0)
	if d.ShouldGrabPack {
		t.Fatal("expected not to grab when <50% wanted")
	}
	if d.Reason != "<50% of episodes wanted — prefer individual" {
		t.Fatalf("unexpected reason: %q", d.Reason)
	}

	noRange := DecidePack(DetectedPack{IsPack: true, Type: PackTypeSingleSeason, SeasonStart: 2, SeasonEnd: 2}, wanted, 0, 0)
	if noRange.Reason != "no episodes in pack range" {
		t.Fatalf("unexpected no-range reason: %q", noRange.Reason)
	}
}

func TestEpisodesFromPack(t *testing.T) {
	t.Parallel()
	packRange := DetectedPack{IsPack: true, Type: PackTypeEpisodeRange, SeasonStart: 3, SeasonEnd: 3, EpisodeStart: 4, EpisodeEnd: 6}
	got := EpisodesFromPack(packRange, 3, 10)
	want := []int{4, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("episode[%d] = %d, want %d", i, got[i], want[i])
		}
	}

	fullSeason := DetectedPack{IsPack: true, Type: PackTypeSingleSeason, SeasonStart: 1, SeasonEnd: 1}
	got = EpisodesFromPack(fullSeason, 1, 4)
	if len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Fatalf("unexpected full-season episodes: %+v", got)
	}

	if out := EpisodesFromPack(fullSeason, 2, 4); out != nil {
		t.Fatalf("expected nil for season outside pack, got %+v", out)
	}
}
