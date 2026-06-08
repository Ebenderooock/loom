package music

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/storage"
)

// fakeProvider is an in-memory metadata.MusicMetadataProvider for tests.
type fakeProvider struct {
	artist   *metadata.ArtistMetadata
	albums   []*metadata.AlbumMetadata
	releases map[string][]*metadata.AlbumReleaseMetadata // album mbid -> editions
	tracks   map[string]*metadata.AlbumReleaseMetadata   // release mbid -> edition w/ tracks
}

func (f *fakeProvider) Name() string { return "musicbrainz" }

func (f *fakeProvider) SearchArtist(_ context.Context, _ string, _ int) ([]*metadata.ArtistMetadata, error) {
	return []*metadata.ArtistMetadata{f.artist}, nil
}

func (f *fakeProvider) GetArtist(_ context.Context, mbid string) (*metadata.ArtistMetadata, error) {
	if f.artist != nil && f.artist.MBID == mbid {
		return f.artist, nil
	}
	return nil, nil
}

func (f *fakeProvider) GetArtistAlbums(_ context.Context, _ string) ([]*metadata.AlbumMetadata, error) {
	return f.albums, nil
}

func (f *fakeProvider) GetAlbum(_ context.Context, rgMBID string) (*metadata.AlbumMetadata, []*metadata.AlbumReleaseMetadata, error) {
	for _, a := range f.albums {
		if a.MBID == rgMBID {
			return a, f.releases[rgMBID], nil
		}
	}
	return nil, nil, nil
}

func (f *fakeProvider) GetAlbumRelease(_ context.Context, releaseMBID string) (*metadata.AlbumReleaseMetadata, error) {
	return f.tracks[releaseMBID], nil
}

func newTestService(t *testing.T, prov metadata.MusicMetadataProvider) (Service, Repository) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(dir, "music_test.db")},
	}
	db, err := storage.Open(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewRepository(db.DB())
	return NewService(repo, prov, nil), repo
}

func sampleProvider() *fakeProvider {
	return &fakeProvider{
		artist: &metadata.ArtistMetadata{MBID: "artist-1", Name: "Test Artist", Country: "GB", Genres: []string{"rock"}},
		albums: []*metadata.AlbumMetadata{
			{MBID: "rg-1", Title: "First Album", ArtistMBID: "artist-1", Type: "Album", ReleaseDate: "2001-01-01"},
			{MBID: "rg-2", Title: "Second Album", ArtistMBID: "artist-1", Type: "Album", ReleaseDate: "2005-06-01"},
		},
		releases: map[string][]*metadata.AlbumReleaseMetadata{
			"rg-1": {
				{MBID: "rel-1a", Title: "First Album", Status: "Official", TrackCount: 2, MediaCount: 1},
				{MBID: "rel-1b", Title: "First Album (Promo)", Status: "Promotion", TrackCount: 2, MediaCount: 1},
			},
		},
		tracks: map[string]*metadata.AlbumReleaseMetadata{
			"rel-1a": {MBID: "rel-1a", Title: "First Album", Tracks: []metadata.TrackMetadata{
				{MBID: "rec-1", TrackID: "trk-1", Title: "Song One", TrackNumber: 1, DiscNumber: 1, DurationMs: 180000},
				{MBID: "rec-2", TrackID: "trk-2", Title: "Song Two", TrackNumber: 2, DiscNumber: 1, DurationMs: 200000},
			}},
		},
	}
}

func TestAddArtistPersistsArtistAndAlbums(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, sampleProvider())

	artist, err := svc.AddArtist(ctx, AddArtistRequest{MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1"})
	if err != nil {
		t.Fatalf("AddArtist: %v", err)
	}
	if artist.MBID != "artist-1" || artist.Name != "Test Artist" {
		t.Fatalf("unexpected artist: %+v", artist)
	}
	if artist.MonitoringStatus != MonitoringMonitored {
		t.Errorf("monitoring = %q, want monitored", artist.MonitoringStatus)
	}
	if len(artist.Albums) != 2 {
		t.Fatalf("albums = %d, want 2", len(artist.Albums))
	}
	for _, al := range artist.Albums {
		if !al.Monitored {
			t.Errorf("album %q should be monitored", al.Title)
		}
	}
	if artist.Stats == nil || artist.Stats.AlbumCount != 2 {
		t.Errorf("stats AlbumCount = %v, want 2", artist.Stats)
	}
}

func TestAddArtistIsIdempotentAndRevives(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestService(t, sampleProvider())

	first, err := svc.AddArtist(ctx, AddArtistRequest{MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1"})
	if err != nil {
		t.Fatalf("AddArtist #1: %v", err)
	}
	// Soft-delete then re-add: should revive the same row, not duplicate.
	if err := svc.DeleteArtist(ctx, first.ID); err != nil {
		t.Fatalf("DeleteArtist: %v", err)
	}
	second, err := svc.AddArtist(ctx, AddArtistRequest{MBID: "artist-1", QualityProfileID: "qp-2", LibraryID: "lib-1"})
	if err != nil {
		t.Fatalf("AddArtist #2: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("revive should reuse id: got %q want %q", second.ID, first.ID)
	}
	if second.QualityProfileID != "qp-2" {
		t.Errorf("revive should update fields: qp = %q", second.QualityProfileID)
	}
	all, err := repo.ListArtists(ctx)
	if err != nil {
		t.Fatalf("ListArtists: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("artist count = %d, want 1", len(all))
	}
}

func TestGetAlbumLazilyFetchesReleasesAndTracks(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, sampleProvider())

	artist, err := svc.AddArtist(ctx, AddArtistRequest{MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1"})
	if err != nil {
		t.Fatalf("AddArtist: %v", err)
	}
	var albumID string
	for _, al := range artist.Albums {
		if al.MBID == "rg-1" {
			albumID = al.ID
		}
	}
	if albumID == "" {
		t.Fatal("album rg-1 not found")
	}

	album, err := svc.GetAlbum(ctx, albumID)
	if err != nil {
		t.Fatalf("GetAlbum: %v", err)
	}
	if len(album.Releases) != 2 {
		t.Fatalf("releases = %d, want 2", len(album.Releases))
	}
	if album.SelectedReleaseID == "" {
		t.Fatal("expected a selected release")
	}
	// The official edition (rel-1a) should be selected over the promo.
	var selectedMBID string
	for _, rel := range album.Releases {
		if rel.ID == album.SelectedReleaseID {
			selectedMBID = rel.MBID
		}
	}
	if selectedMBID != "rel-1a" {
		t.Errorf("selected release mbid = %q, want rel-1a", selectedMBID)
	}
	if len(album.Tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(album.Tracks))
	}
	if album.Tracks[0].Title != "Song One" || album.Tracks[0].TrackMBID != "trk-1" {
		t.Errorf("unexpected first track: %+v", album.Tracks[0])
	}
	if album.TracksFetchedAt == nil {
		t.Error("TracksFetchedAt should be set after fetch")
	}

	// Second GetAlbum must not re-fetch (idempotent track count).
	album2, err := svc.GetAlbum(ctx, albumID)
	if err != nil {
		t.Fatalf("GetAlbum #2: %v", err)
	}
	if len(album2.Tracks) != 2 {
		t.Errorf("tracks after re-get = %d, want 2", len(album2.Tracks))
	}
}

func TestLookupArtistsMarksAlreadyAdded(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, sampleProvider())

	before, err := svc.LookupArtists(ctx, "test", 10)
	if err != nil {
		t.Fatalf("LookupArtists: %v", err)
	}
	if len(before) != 1 || before[0].AlreadyAdded {
		t.Fatalf("before add: %+v", before)
	}
	if _, err := svc.AddArtist(ctx, AddArtistRequest{MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1"}); err != nil {
		t.Fatalf("AddArtist: %v", err)
	}
	after, err := svc.LookupArtists(ctx, "test", 10)
	if err != nil {
		t.Fatalf("LookupArtists #2: %v", err)
	}
	if len(after) != 1 || !after[0].AlreadyAdded {
		t.Fatalf("after add: %+v", after)
	}
}
