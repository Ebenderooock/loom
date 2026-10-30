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

func TestSharedAlbumIsLinkedToEveryCreditedArtist(t *testing.T) {
	ctx := context.Background()
	firstProvider := sampleProvider()
	firstService, repo := newTestService(t, firstProvider)
	first, err := firstService.AddArtist(ctx, AddArtistRequest{
		MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1",
	})
	if err != nil {
		t.Fatalf("add first artist: %v", err)
	}

	secondProvider := &fakeProvider{
		artist: &metadata.ArtistMetadata{MBID: "artist-2", Name: "Second Artist"},
		albums: []*metadata.AlbumMetadata{
			{MBID: "rg-1", Title: "First Album", ArtistMBID: "artist-2", Type: "Album"},
		},
	}
	secondService := NewService(repo, secondProvider, nil)
	second, err := secondService.AddArtist(ctx, AddArtistRequest{
		MBID: "artist-2", QualityProfileID: "qp-1", LibraryID: "lib-1",
	})
	if err != nil {
		t.Fatalf("add second artist: %v", err)
	}

	firstAlbums, err := secondService.ListAlbumsByArtist(ctx, first.ID)
	if err != nil {
		t.Fatalf("list first artist albums: %v", err)
	}
	secondAlbums, err := secondService.ListAlbumsByArtist(ctx, second.ID)
	if err != nil {
		t.Fatalf("list second artist albums: %v", err)
	}
	if len(firstAlbums) != 2 || len(secondAlbums) != 1 {
		t.Fatalf("album counts = first %d, second %d; want 2 and 1", len(firstAlbums), len(secondAlbums))
	}
	if firstAlbums[0].ID != secondAlbums[0].ID {
		t.Fatalf("shared album IDs differ: %q and %q", firstAlbums[0].ID, secondAlbums[0].ID)
	}
	if second.Stats == nil || second.Stats.AlbumCount != 1 {
		t.Fatalf("second artist stats = %+v, want one shared album", second.Stats)
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

func TestSelectAlbumReleaseValidatesOwnershipAndRefreshesTracks(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, sampleProvider())
	artist, err := svc.AddArtist(ctx, AddArtistRequest{
		MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1",
	})
	if err != nil {
		t.Fatalf("AddArtist: %v", err)
	}
	album, err := svc.GetAlbum(ctx, artist.Albums[0].ID)
	if err != nil {
		t.Fatalf("GetAlbum: %v", err)
	}
	var otherRelease *AlbumRelease
	for _, release := range album.Releases {
		if release.ID != album.SelectedReleaseID {
			otherRelease = release
			break
		}
	}
	if otherRelease == nil {
		t.Fatal("expected another release")
	}

	selected, err := svc.SelectAlbumRelease(ctx, album.ID, otherRelease.ID)
	if err != nil {
		t.Fatalf("SelectAlbumRelease: %v", err)
	}
	if selected.SelectedReleaseID != otherRelease.ID {
		t.Fatalf("selected release = %q, want %q", selected.SelectedReleaseID, otherRelease.ID)
	}
	if _, err := svc.SelectAlbumRelease(ctx, album.ID, "release-from-another-album"); err == nil {
		t.Fatal("expected invalid release ownership to be rejected")
	}
}

func TestPickReleaseHonorsStatusAndEarliestDate(t *testing.T) {
	releases := []*AlbumRelease{
		{ID: "late", Status: "Official", ReleaseDate: "2010-01-01", TrackCount: 30},
		{ID: "early", Status: "Official", ReleaseDate: "2000-01-01", TrackCount: 10},
		{ID: "preferred", Status: "Promotion", ReleaseDate: "2020-01-01", TrackCount: 2},
	}
	got := pickRelease(releases, []string{"Promotion", "Official"})
	if got.ID != "preferred" {
		t.Fatalf("pickRelease() = %q, want preferred status", got.ID)
	}
	got = pickRelease(releases[:2], []string{"Official"})
	if got.ID != "early" {
		t.Fatalf("pickRelease() = %q, want earliest date", got.ID)
	}
}

func TestMusicProfilesCanBeManagedWithoutDeletingAssignedProfiles(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, sampleProvider())
	audio, err := svc.CreateAudioQualityProfile(ctx, CreateAudioQualityProfileRequest{
		Name: "Portable", Items: []AudioProfileItem{{DefinitionID: "aq_mp3_320", Allowed: true}},
		Cutoff: "aq_mp3_320",
	})
	if err != nil {
		t.Fatalf("CreateAudioQualityProfile: %v", err)
	}
	metadata, err := svc.CreateMetadataProfile(ctx, CreateMetadataProfileRequest{
		Name: "Albums only", PrimaryTypes: []string{"Album"}, ReleaseStatuses: []string{"official"},
	})
	if err != nil {
		t.Fatalf("CreateMetadataProfile: %v", err)
	}
	if _, err := svc.AddArtist(ctx, AddArtistRequest{
		MBID: "artist-1", QualityProfileID: audio.ID, MetadataProfileID: metadata.ID, LibraryID: "lib-1",
	}); err != nil {
		t.Fatalf("AddArtist with new profiles: %v", err)
	}
	if err := svc.DeleteAudioQualityProfile(ctx, audio.ID); err == nil {
		t.Fatal("expected assigned audio profile deletion to fail")
	}
	if err := svc.DeleteMetadataProfile(ctx, metadata.ID); err == nil {
		t.Fatal("expected assigned metadata profile deletion to fail")
	}

	newName := "Official albums"
	update := UpdateMetadataProfileRequest{Name: &newName}
	updated, err := svc.UpdateMetadataProfile(ctx, metadata.ID, update)
	if err != nil {
		t.Fatalf("UpdateMetadataProfile: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("updated metadata profile name = %q, want %q", updated.Name, newName)
	}
}

func TestAddArtistHonorsSearchOnAddHookAndValidatesMonitoring(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, sampleProvider())
	if _, err := svc.AddArtist(ctx, AddArtistRequest{
		MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1",
		MonitoringStatus: "sometimes",
	}); err == nil {
		t.Fatal("expected invalid monitoring status to fail")
	}
	concrete, ok := svc.(*service)
	if !ok {
		t.Fatal("service is not the expected implementation")
	}
	calls := 0
	concrete.SetSearchOnAdd(func(_ context.Context, artistID string) error {
		calls++
		if artistID == "" {
			t.Error("search hook received empty artist ID")
		}
		return nil
	})
	if _, err := svc.AddArtist(ctx, AddArtistRequest{
		MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1", Search: true,
	}); err != nil {
		t.Fatalf("AddArtist: %v", err)
	}
	if calls != 1 {
		t.Fatalf("search-on-add calls = %d, want 1", calls)
	}
}

func TestTrackRefreshPreservesPhysicalFileAssociations(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestService(t, sampleProvider())
	artist, err := svc.AddArtist(ctx, AddArtistRequest{
		MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	album := artist.Albums[0]
	loaded, err := svc.GetAlbum(ctx, album.ID)
	if err != nil {
		t.Fatal(err)
	}
	track := loaded.Tracks[0]
	if err := svc.ImportTrackFile(ctx, &TrackFile{
		ID: "file-1", TrackID: track.ID, AlbumID: loaded.ID, ArtistID: artist.ID,
		FilePath: "/library/track.flac", Format: "FLAC",
	}); err != nil {
		t.Fatal(err)
	}
	loaded.TracksFetchedAt = nil
	if err := repo.UpdateAlbum(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	refreshed, err := svc.GetAlbum(ctx, loaded.ID)
	if err != nil {
		t.Fatal(err)
	}
	var refreshedTrack *Track
	for _, candidate := range refreshed.Tracks {
		if candidate.ID == track.ID {
			refreshedTrack = candidate
		}
	}
	if len(refreshed.Tracks) != 2 || refreshedTrack == nil || !refreshedTrack.HasFile {
		t.Fatalf("refreshed tracks lost stable ID/file state: %+v", refreshed.Tracks)
	}
	files, err := repo.ListTrackFilesByArtist(ctx, artist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].TrackID != track.ID {
		t.Fatalf("track files after refresh = %+v, want preserved association", files)
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

func TestAlbumMatchesMetadataProfile(t *testing.T) {
	profile := &MetadataProfile{
		PrimaryTypes:   StringSlice{"Album", "EP"},
		SecondaryTypes: StringSlice{"Live"},
	}
	cases := []struct {
		name      string
		primary   string
		secondary []string
		profile   *MetadataProfile
		want      bool
	}{
		{"nil profile allows all", "Single", []string{"Compilation"}, nil, true},
		{"allowed primary, no secondary", "Album", nil, profile, true},
		{"allowed primary lowercase", "album", nil, profile, true},
		{"disallowed primary", "Single", nil, profile, false},
		{"allowed secondary", "Album", []string{"Live"}, profile, true},
		{"disallowed secondary", "Album", []string{"Compilation"}, profile, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := albumMatchesMetadataProfile(c.primary, c.secondary, c.profile); got != c.want {
				t.Errorf("albumMatchesMetadataProfile(%q,%v) = %v, want %v", c.primary, c.secondary, got, c.want)
			}
		})
	}
}
