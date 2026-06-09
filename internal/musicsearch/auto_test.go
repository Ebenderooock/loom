package musicsearch

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/music"
)

// fakeRepo embeds music.Repository so only the methods exercised by the
// auto-searcher need real implementations; any other call panics (nil iface).
type fakeRepo struct {
	music.Repository
	artists map[string][]*music.Album // artistID -> albums
	tracks  map[string][]*music.Track // albumID -> tracks
	artist  []*music.Artist
}

func (f *fakeRepo) ListArtists(context.Context) ([]*music.Artist, error) {
	return f.artist, nil
}

func (f *fakeRepo) ListAlbumsByArtist(_ context.Context, artistID string) ([]*music.Album, error) {
	return f.artists[artistID], nil
}

func (f *fakeRepo) ListTracksByAlbum(_ context.Context, albumID string) ([]*music.Track, error) {
	return f.tracks[albumID], nil
}

func trackList(present ...bool) []*music.Track {
	out := make([]*music.Track, 0, len(present))
	for _, p := range present {
		out = append(out, &music.Track{Monitored: true, HasFile: p})
	}
	return out
}

func TestMissingAlbumCandidates(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)

	repo := &fakeRepo{
		artist: []*music.Artist{
			{ID: "a1", Name: "Monitored", MonitoringStatus: music.MonitoringMonitored},
			{ID: "a2", Name: "Unmonitored", MonitoringStatus: music.MonitoringUnmonitored},
		},
		artists: map[string][]*music.Album{
			"a1": {
				{ID: "missing", Monitored: true, LastSearchAt: &old},      // due, missing track
				{ID: "complete", Monitored: true, LastSearchAt: &old},     // all present -> skip
				{ID: "unmonitored", Monitored: false, LastSearchAt: &old}, // skip
				{ID: "recent", Monitored: true, LastSearchAt: &recent},    // within recheck -> skip
				{ID: "never", Monitored: true},                            // never searched, missing
			},
			"a2": {
				{ID: "other", Monitored: true, LastSearchAt: &old},
			},
		},
		tracks: map[string][]*music.Track{
			"missing":  trackList(true, false),
			"complete": trackList(true, true),
			"recent":   trackList(false),
			"never":    trackList(false),
			"other":    trackList(false),
		},
	}

	eng := NewEngine(nil, nil, repo, slog.Default())
	got, err := eng.MissingAlbumCandidates(context.Background(), 12*time.Hour)
	if err != nil {
		t.Fatalf("MissingAlbumCandidates: %v", err)
	}

	ids := map[string]bool{}
	for _, al := range got {
		ids[al.ID] = true
	}
	if !ids["missing"] || !ids["never"] {
		t.Errorf("expected missing+never, got %v", ids)
	}
	if ids["complete"] || ids["unmonitored"] || ids["recent"] || ids["other"] {
		t.Errorf("unexpected candidate included: %v", ids)
	}

	// "never" (nil LastSearchAt) should sort before "missing" (older timestamp).
	if len(got) >= 2 && got[0].ID != "never" {
		t.Errorf("expected never-searched album first, got %q", got[0].ID)
	}
}

func TestAlbumMissingTracksNoListing(t *testing.T) {
	repo := &fakeRepo{tracks: map[string][]*music.Track{"x": nil}}
	eng := NewEngine(nil, nil, repo, slog.Default())
	missing, err := eng.albumMissingTracks(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if !missing {
		t.Error("album with no track listing should be treated as missing")
	}
}
