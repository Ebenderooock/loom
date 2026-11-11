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

func TestMatchTrack_TitleQualifierFallback(t *testing.T) {
	single := []*music.Track{
		{ID: "heavy", Title: "Heavy", TrackNumber: 1, DiscNumber: 1},
		{ID: "remix", Title: "Heavy (Nicky Romero remix)", TrackNumber: 2, DiscNumber: 1},
	}
	if got := matchTrack(single, &music.AudioTags{Title: "Heavy (feat. Kiiara)"}); got == nil || got.ID != "heavy" {
		t.Fatalf("featured-artist tag should match Heavy, got %+v", got)
	}
	cd := []*music.Track{
		{ID: "album", Title: "Heavy (album version)", TrackNumber: 1, DiscNumber: 1},
		{ID: "inst", Title: "Heavy (instrumental)", TrackNumber: 2, DiscNumber: 1},
	}
	if got := matchTrack(cd, &music.AudioTags{Title: "Heavy"}); got != nil {
		t.Fatalf("ambiguous base titles must not match, got %+v", got)
	}
	if got := matchTrack(cd[:1], &music.AudioTags{Title: "Heavy feat. Kiiara"}); got == nil || got.ID != "album" {
		t.Fatalf("unique base title should match, got %+v", got)
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

func TestMusicScannerNormalizationPreservesUnicodeNames(t *testing.T) {
	if got := normalizeTitle("Beyoncé"); got != "beyonce" {
		t.Fatalf("normalizeTitle(Beyoncé) = %q, want beyonce", got)
	}
	if got := normalizeTitle("東京"); got != "東京" {
		t.Fatalf("normalizeTitle(東京) = %q, want 東京", got)
	}
}
