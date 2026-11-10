package musicsearch

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/music"
)

// upgradeRepo extends the behaviour needed for cutoff-unmet upgrade detection.
type upgradeRepo struct {
	fakeRepo
	defs       []*music.AudioQualityDefinition
	profiles   map[string]*music.AudioQualityProfile
	trackFiles map[string][]*music.TrackFile // artistID -> files
	albumFiles map[string][]*music.TrackFile // albumID -> files
}

func (r *upgradeRepo) ListAudioQualityDefinitions(context.Context) ([]*music.AudioQualityDefinition, error) {
	return r.defs, nil
}

func (r *upgradeRepo) GetAudioQualityProfile(_ context.Context, id string) (*music.AudioQualityProfile, error) {
	return r.profiles[id], nil
}

func (r *upgradeRepo) ListTrackFilesByArtist(_ context.Context, artistID string) ([]*music.TrackFile, error) {
	return r.trackFiles[artistID], nil
}

func (r *upgradeRepo) ListTrackFilesByAlbum(_ context.Context, albumID string) ([]*music.TrackFile, error) {
	if files, ok := r.albumFiles[albumID]; ok {
		return files, nil
	}
	var files []*music.TrackFile
	for _, artistFiles := range r.trackFiles {
		for _, file := range artistFiles {
			if file.AlbumID == albumID {
				files = append(files, file)
			}
		}
	}
	return files, nil
}

func TestCutoffUnmetCandidates(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour)
	defs := []*music.AudioQualityDefinition{
		{ID: "aq_mp3_320", Name: "MP3 320", Format: "mp3", Bitrate: 320, TierOrder: 4},
		{ID: "aq_flac", Name: "FLAC", Format: "flac", Lossless: true, TierOrder: 8},
	}

	items, _ := json.Marshal([]music.AudioProfileItem{
		{DefinitionID: "aq_mp3_320", Allowed: true},
		{DefinitionID: "aq_flac", Allowed: true},
	})
	profile := &music.AudioQualityProfile{ID: "p1", Cutoff: "aq_flac", UpgradeAllowed: true, Items: items}

	repo := &upgradeRepo{
		fakeRepo: fakeRepo{
			artist: []*music.Artist{
				{ID: "a1", Name: "Art", MonitoringStatus: music.MonitoringMonitored, QualityProfileID: "p1"},
			},
			artists: map[string][]*music.Album{
				"a1": {
					{ID: "below", ArtistID: "a1", Monitored: true, LastSearchAt: &old},    // mp3 -> below FLAC cutoff
					{ID: "atcutoff", ArtistID: "a1", Monitored: true, LastSearchAt: &old}, // flac -> meets cutoff
					{ID: "incomplete", ArtistID: "a1", Monitored: true, LastSearchAt: &old},
					{ID: "shared-other", ArtistID: "a2", Monitored: true, LastSearchAt: &old},
				},
			},
			tracks: map[string][]*music.Track{
				"below":        trackList(true),
				"atcutoff":     trackList(true),
				"incomplete":   trackList(false), // missing -> not an upgrade candidate
				"shared-other": trackList(true),
			},
		},
		defs:     defs,
		profiles: map[string]*music.AudioQualityProfile{"p1": profile},
		trackFiles: map[string][]*music.TrackFile{
			"a1": {
				{AlbumID: "below", Format: "mp3", Bitrate: 320},
				{AlbumID: "atcutoff", Format: "flac"},
				{AlbumID: "shared-other", Format: "mp3", Bitrate: 320},
			},
		},
	}

	eng := NewEngine(nil, nil, repo, slog.Default())
	got, err := eng.CutoffUnmetCandidates(context.Background(), 12*time.Hour)
	if err != nil {
		t.Fatalf("CutoffUnmetCandidates: %v", err)
	}
	ids := map[string]bool{}
	for _, al := range got {
		ids[al.ID] = true
	}
	if !ids["below"] {
		t.Errorf("expected below-cutoff album to be an upgrade candidate, got %v", ids)
	}
	if ids["atcutoff"] || ids["incomplete"] || ids["shared-other"] {
		t.Errorf("unexpected candidate: %v", ids)
	}
}

func TestCutoffUnmetCandidatesIncludesStandaloneAlbumProfile(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour)
	standalone := &music.Album{
		ID: "standalone-upgrade", Monitored: true, LastSearchAt: &old,
		QualityProfileID: "album-profile",
	}
	defs := []*music.AudioQualityDefinition{
		{ID: "aq_mp3_320", Name: "MP3 320", Format: "mp3", Bitrate: 320, TierOrder: 4},
		{ID: "aq_flac", Name: "FLAC", Format: "flac", Lossless: true, TierOrder: 8},
	}
	profile := &music.AudioQualityProfile{
		ID: "album-profile", Cutoff: "aq_flac", UpgradeAllowed: true,
	}
	artistProfile := &music.AudioQualityProfile{
		ID: "artist-profile", Cutoff: "aq_flac", UpgradeAllowed: false,
	}
	repo := &upgradeRepo{
		fakeRepo: fakeRepo{
			artist: []*music.Artist{
				{ID: "contributor", Name: "Contributor", MonitoringStatus: music.MonitoringMonitored, QualityProfileID: artistProfile.ID},
			},
			artists:   map[string][]*music.Album{"contributor": {standalone}},
			allAlbums: []*music.Album{standalone},
			tracks:    map[string][]*music.Track{standalone.ID: trackList(true)},
		},
		defs:     defs,
		profiles: map[string]*music.AudioQualityProfile{profile.ID: profile, artistProfile.ID: artistProfile},
		albumFiles: map[string][]*music.TrackFile{
			standalone.ID: {{AlbumID: standalone.ID, Format: "mp3", Bitrate: 320}},
		},
	}
	eng := NewEngine(nil, nil, repo, slog.Default())
	got, err := eng.CutoffUnmetCandidates(context.Background(), 12*time.Hour)
	if err != nil {
		t.Fatalf("CutoffUnmetCandidates: %v", err)
	}
	if len(got) != 1 || got[0].ID != standalone.ID {
		t.Fatalf("standalone upgrade candidates = %+v, want %q", got, standalone.ID)
	}
}
