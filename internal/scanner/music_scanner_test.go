package scanner

import (
	"testing"

	"github.com/ebenderooock/loom/internal/music"
)

func TestMatchTrack_ByNumber(t *testing.T) {
	tracks := []*music.Track{
		{ID: "a", Title: "Intro", TrackNumber: 1, DiscNumber: 1},
		{ID: "b", Title: "Money", TrackNumber: 6, DiscNumber: 1},
		{ID: "c", Title: "Money", TrackNumber: 6, DiscNumber: 2},
	}
	got := matchTrack(tracks, &music.AudioTags{TrackNumber: 6, DiscNumber: 2, Title: "Money"})
	if got == nil || got.ID != "c" {
		t.Fatalf("expected track c (disc 2), got %+v", got)
	}
}

func TestMatchTrack_DefaultDisc(t *testing.T) {
	tracks := []*music.Track{
		{ID: "a", Title: "Money", TrackNumber: 6, DiscNumber: 1},
	}
	// Tag has no disc number (0) → should default to disc 1.
	got := matchTrack(tracks, &music.AudioTags{TrackNumber: 6, DiscNumber: 0})
	if got == nil || got.ID != "a" {
		t.Fatalf("expected track a, got %+v", got)
	}
}

func TestMatchTrack_TitleFallback(t *testing.T) {
	tracks := []*music.Track{
		{ID: "a", Title: "Time", TrackNumber: 4, DiscNumber: 1},
	}
	// No track number → fall back to normalized title match.
	got := matchTrack(tracks, &music.AudioTags{Title: "the time"})
	if got == nil || got.ID != "a" {
		t.Fatalf("expected title fallback to track a, got %+v", got)
	}
}

func TestMatchTrack_NoMatch(t *testing.T) {
	tracks := []*music.Track{
		{ID: "a", Title: "Time", TrackNumber: 4, DiscNumber: 1},
	}
	if got := matchTrack(tracks, &music.AudioTags{TrackNumber: 9, Title: "Unknown"}); got != nil {
		t.Fatalf("expected no match, got %+v", got)
	}
	if got := matchTrack(nil, &music.AudioTags{TrackNumber: 1}); got != nil {
		t.Fatalf("expected nil for empty tracks, got %+v", got)
	}
}
