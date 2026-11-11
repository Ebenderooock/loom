package music

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/storage"
)

// fakeProvider is an in-memory metadata.MusicMetadataProvider for tests.
type fakeProvider struct {
	artist     *metadata.ArtistMetadata
	albumErr   error
	releaseErr error
	albums     []*metadata.AlbumMetadata
	releases   map[string][]*metadata.AlbumReleaseMetadata // album mbid -> editions
	tracks     map[string]*metadata.AlbumReleaseMetadata   // release mbid -> edition w/ tracks
}

func (f *fakeProvider) Name() string { return "musicbrainz" }

func (f *fakeProvider) SearchArtist(_ context.Context, _ string, _ int) ([]*metadata.ArtistMetadata, error) {
	return []*metadata.ArtistMetadata{f.artist}, nil
}

func (f *fakeProvider) SearchAlbum(_ context.Context, _ string, _ int) ([]*metadata.AlbumMetadata, error) {
	return f.albums, nil
}

func (f *fakeProvider) GetArtist(_ context.Context, mbid string) (*metadata.ArtistMetadata, error) {
	if f.artist != nil && f.artist.MBID == mbid {
		return f.artist, nil
	}
	return nil, nil
}

func (f *fakeProvider) GetArtistAlbums(_ context.Context, _ string) ([]*metadata.AlbumMetadata, error) {
	return f.albums, f.albumErr
}

func TestRefreshArtistCatalogPreservesExcludedFiles(t *testing.T) {
	ctx := context.Background()
	provider := sampleProvider()
	svc, repo := newTestService(t, provider)
	artist, err := svc.AddArtist(ctx, AddArtistRequest{
		MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	excluded := artist.Albums[1]
	path := filepath.Join(t.TempDir(), "existing.flac")
	if err := os.WriteFile(path, []byte("existing audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceTracks(ctx, excluded.ID, []*Track{{
		ID: "existing-track", AlbumID: excluded.ID, TrackNumber: 1, Monitored: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTrackFile(ctx, &TrackFile{
		ID: "existing-file", TrackID: "existing-track", AlbumID: excluded.ID,
		ArtistID: artist.ID, FilePath: path,
	}); err != nil {
		t.Fatal(err)
	}
	// The refreshed provider catalogue no longer includes this legacy entry.
	provider.albums = provider.albums[:1]
	if _, err := svc.RefreshArtistAlbums(ctx, artist.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.GetArtist(ctx, artist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Albums) != 1 || loaded.Stats.AlbumCount != 1 || loaded.Stats.TrackCount != 0 {
		t.Fatalf("excluded entry still affects catalogue/stats: %+v", loaded)
	}
	artists, err := svc.ListArtists(ctx)
	if err != nil || len(artists) != 1 || artists[0].Stats.AlbumCount != 1 {
		t.Fatalf("artist list stats: artists=%+v, error=%v", artists, err)
	}
	tracks, err := repo.ListTracksByAlbum(ctx, excluded.ID)
	if err != nil || len(tracks) != 1 || !tracks[0].HasFile {
		t.Fatalf("existing track lost: tracks=%+v, error=%v", tracks, err)
	}
	file, err := repo.GetTrackFileByPath(ctx, path)
	if err != nil || file == nil || file.ID != "existing-file" {
		t.Fatalf("existing file record lost: file=%+v, error=%v", file, err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "existing audio" {
		t.Fatalf("existing audio changed: %q, error=%v", content, err)
	}
	// Re-including a group must reuse its album and file records.
	provider.albums = append(provider.albums, &metadata.AlbumMetadata{MBID: excluded.MBID})
	if _, err := svc.RefreshArtistAlbums(ctx, artist.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err = svc.GetArtist(ctx, artist.ID)
	if err != nil || len(loaded.Albums) != 2 || loaded.Albums[1].ID != excluded.ID {
		t.Fatalf("existing album not restored: artist=%+v, error=%v", loaded, err)
	}
}

func TestRefreshArtistCatalogFailurePreservesVisibility(t *testing.T) {
	ctx := context.Background()
	provider := sampleProvider()
	svc, _ := newTestService(t, provider)
	artist, err := svc.AddArtist(ctx, AddArtistRequest{
		MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.albums = nil
	provider.albumErr = errors.New("metadata unavailable")
	if _, err := svc.RefreshArtistAlbums(ctx, artist.ID); err == nil {
		t.Fatal("expected metadata failure")
	}
	loaded, err := svc.GetArtist(ctx, artist.ID)
	if err != nil || len(loaded.Albums) != 2 {
		t.Fatalf("failed refresh changed catalogue: artist=%+v, error=%v", loaded, err)
	}
}

func TestAddAlbumKeepsCreditsWithoutCreatingManagedArtists(t *testing.T) {
	ctx := context.Background()
	provider := &fakeProvider{
		albums: []*metadata.AlbumMetadata{{
			MBID: "compilation-1", Title: "A Shared Record", Type: "Album",
			SecondaryTypes: []string{"Compilation"},
			Artists: []metadata.AlbumArtistMetadata{
				{ArtistMBID: "contributor-1", Name: "First Artist"},
				{ArtistMBID: "contributor-2", Name: "Second Artist"},
			},
		}},
	}
	svc, repo := newTestService(t, provider)
	album, err := svc.AddAlbum(ctx, AddAlbumRequest{
		MBID: "compilation-1", QualityProfileID: "qp-1", LibraryID: "lib-1",
	})
	if err != nil {
		t.Fatalf("AddAlbum: %v", err)
	}
	if album.ArtistID != "" {
		t.Fatalf("standalone album has a primary managed artist: %q", album.ArtistID)
	}
	if len(album.Artists) != 2 || album.Artists[0].Name != "First Artist" ||
		album.Artists[1].Name != "Second Artist" {
		t.Fatalf("artist credits were not preserved: %+v", album.Artists)
	}
	if album.Artists[0].ArtistID != "" || album.Artists[1].ArtistID != "" {
		t.Fatalf("unmanaged contributors were linked as managed artists: %+v", album.Artists)
	}
	artists, err := repo.ListArtists(ctx)
	if err != nil || len(artists) != 0 {
		t.Fatalf("adding a compilation created artists: artists=%+v error=%v", artists, err)
	}
	albums, err := svc.ListAlbums(ctx)
	if err != nil || len(albums) != 1 || albums[0].LibraryID != "lib-1" {
		t.Fatalf("standalone album missing from library listing: albums=%+v error=%v", albums, err)
	}
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
	if f.releaseErr != nil {
		return nil, f.releaseErr
	}
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
	svc, repo := newTestService(t, sampleProvider())
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
	standaloneAudio, err := svc.CreateAudioQualityProfile(ctx, CreateAudioQualityProfileRequest{
		Name: "Standalone quality", Items: []AudioProfileItem{{DefinitionID: "aq_mp3_320", Allowed: true}},
	})
	if err != nil {
		t.Fatalf("CreateAudioQualityProfile for standalone album: %v", err)
	}
	standaloneMetadata, err := svc.CreateMetadataProfile(ctx, CreateMetadataProfileRequest{Name: "Standalone metadata"})
	if err != nil {
		t.Fatalf("CreateMetadataProfile for standalone album: %v", err)
	}
	if _, err := svc.AddArtist(ctx, AddArtistRequest{
		MBID: "artist-1", QualityProfileID: audio.ID, MetadataProfileID: metadata.ID, LibraryID: "lib-1",
	}); err != nil {
		t.Fatalf("AddArtist with new profiles: %v", err)
	}
	if err := repo.CreateAlbum(ctx, &Album{
		ID: "standalone-profile-assignment", MBID: "standalone-profile-assignment",
		Title: "Standalone Compilation", Monitored: true,
		QualityProfileID: standaloneAudio.ID, MetadataProfileID: standaloneMetadata.ID,
	}); err != nil {
		t.Fatalf("create standalone profile assignment: %v", err)
	}
	if err := svc.DeleteAudioQualityProfile(ctx, audio.ID); err == nil {
		t.Fatal("expected artist/album-assigned audio profile deletion to fail")
	}
	if err := svc.DeleteMetadataProfile(ctx, metadata.ID); err == nil {
		t.Fatal("expected artist/album-assigned metadata profile deletion to fail")
	}
	if err := svc.DeleteAudioQualityProfile(ctx, standaloneAudio.ID); err == nil || !strings.Contains(err.Error(), "album") {
		t.Fatalf("expected standalone album audio profile deletion guard, got %v", err)
	}
	if err := svc.DeleteMetadataProfile(ctx, standaloneMetadata.ID); err == nil || !strings.Contains(err.Error(), "album") {
		t.Fatalf("expected standalone album metadata profile deletion guard, got %v", err)
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

func TestLoadAlbumTracksReportsUnavailableMetadataAndRetries(t *testing.T) {
	ctx := context.Background()
	provider := sampleProvider()
	svc, _ := newTestService(t, provider)

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

	provider.releaseErr = errors.New("musicbrainz busy")
	album, err := svc.GetAlbum(ctx, albumID)
	if err != nil || len(album.Tracks) != 0 {
		t.Fatalf("GetAlbum should degrade to no tracks, got album=%+v err=%v", album, err)
	}
	if _, err := svc.LoadAlbumTracks(ctx, albumID); !errors.Is(err, ErrAlbumTracksUnavailable) ||
		!strings.Contains(err.Error(), "musicbrainz busy") {
		t.Fatalf("LoadAlbumTracks error = %v, want ErrAlbumTracksUnavailable with cause", err)
	}

	provider.releaseErr = nil
	album, err = svc.LoadAlbumTracks(ctx, albumID)
	if err != nil {
		t.Fatalf("LoadAlbumTracks after recovery: %v", err)
	}
	if len(album.Tracks) != 2 {
		t.Fatalf("tracks after recovery = %d, want 2", len(album.Tracks))
	}
}

func TestGetAlbumRetriesEmptyMetadataAndHistoricalEmptyTrackCache(t *testing.T) {
	ctx := context.Background()
	for _, staleTimestamp := range []bool{false, true} {
		t.Run(fmt.Sprint(staleTimestamp), func(t *testing.T) {
			provider := sampleProvider()
			svc, repo := newTestService(t, provider)
			artist, err := svc.AddArtist(ctx, AddArtistRequest{
				MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1",
			})
			if err != nil {
				t.Fatal(err)
			}
			albumID := artist.Albums[0].ID
			fullTracks := provider.tracks["rel-1a"].Tracks
			if staleTimestamp {
				album, err := svc.GetAlbum(ctx, albumID)
				if err != nil {
					t.Fatal(err)
				}
				if err := repo.ReplaceTracks(ctx, albumID, nil); err != nil {
					t.Fatal(err)
				}
				now := time.Now().UTC()
				album.TracksFetchedAt = &now
				if err := repo.UpdateAlbum(ctx, album); err != nil {
					t.Fatal(err)
				}
			}
			provider.tracks["rel-1a"].Tracks = nil
			album, err := svc.GetAlbum(ctx, albumID)
			if err != nil || len(album.Tracks) != 0 || album.TracksFetchedAt != nil {
				t.Fatalf("empty metadata marked as successfully fetched: album=%+v err=%v", album, err)
			}
			if _, err := svc.LoadAlbumTracks(ctx, albumID); !errors.Is(err, ErrAlbumTracksUnavailable) {
				t.Fatalf("empty track metadata must fail import explicitly: %v", err)
			}
			provider.tracks["rel-1a"].Tracks = fullTracks
			album, err = svc.GetAlbum(ctx, albumID)
			if err != nil || len(album.Tracks) != 2 || album.TracksFetchedAt == nil {
				t.Fatalf("GET album did not recover tracks: album=%+v err=%v", album, err)
			}
		})
	}
}

func TestEmptyTrackRefreshPreservesStoredTracks(t *testing.T) {
	ctx := context.Background()
	provider := sampleProvider()
	svc, repo := newTestService(t, provider)
	artist, err := svc.AddArtist(ctx, AddArtistRequest{
		MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	album, err := svc.GetAlbum(ctx, artist.Albums[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	firstTrackID := album.Tracks[0].ID
	album.TracksFetchedAt = nil
	if err := repo.UpdateAlbum(ctx, album); err != nil {
		t.Fatal(err)
	}
	provider.tracks["rel-1a"].Tracks = nil
	album, err = svc.LoadAlbumTracks(ctx, album.ID)
	if err != nil || len(album.Tracks) != 2 || album.Tracks[0].ID != firstTrackID {
		t.Fatalf("empty refresh removed stored track identity: album=%+v err=%v", album, err)
	}
}

func TestTrackMonitoringCascadesAndMonitorsAlbum(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestService(t, sampleProvider())
	artist, err := svc.AddArtist(ctx, AddArtistRequest{MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1"})
	if err != nil {
		t.Fatal(err)
	}
	album, err := svc.GetAlbum(ctx, artist.Albums[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(album.Tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(album.Tracks))
	}

	if _, err := svc.SetAlbumMonitored(ctx, album.ID, false); err != nil {
		t.Fatal(err)
	}
	tracks, _ := repo.ListTracksByAlbum(ctx, album.ID)
	for _, tr := range tracks {
		if tr.Monitored {
			t.Fatalf("track %q still monitored after album unmonitor", tr.Title)
		}
	}

	track := tracks[0]
	updated, err := svc.SetTrackMonitored(ctx, album.ID, track.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Monitored {
		t.Fatal("track should be monitored")
	}
	al, _ := repo.GetAlbum(ctx, album.ID)
	if !al.Monitored {
		t.Fatal("monitoring a track should monitor its album")
	}
	tracks, _ = repo.ListTracksByAlbum(ctx, album.ID)
	if !tracks[0].Monitored || tracks[1].Monitored {
		t.Fatalf("only the toggled track should be monitored: %+v %+v", tracks[0], tracks[1])
	}

	if _, err := svc.SetTrackMonitored(ctx, artist.Albums[1].ID, track.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("track from another album err = %v, want ErrNotFound", err)
	}
	if _, err := svc.SetTrackMonitored(ctx, album.ID, "missing", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing track err = %v, want ErrNotFound", err)
	}
}

func TestArtistFilesAndHistory(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestService(t, sampleProvider())
	artist, err := svc.AddArtist(ctx, AddArtistRequest{MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1"})
	if err != nil {
		t.Fatal(err)
	}
	album, err := svc.GetAlbum(ctx, artist.Albums[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ImportTrackFile(ctx, &TrackFile{
		ID: "file-1", TrackID: album.Tracks[0].ID, AlbumID: album.ID, ArtistID: artist.ID,
		FilePath: "/library/a/01.flac", Format: "FLAC", Size: 1234,
	}); err != nil {
		t.Fatal(err)
	}
	files, err := svc.ListArtistFiles(ctx, artist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].AlbumTitle != album.Title || files[0].TrackTitle != "Song One" || files[0].TrackNumber != 1 {
		t.Fatalf("files = %+v", files)
	}

	db := repo.(*sqlRepo).db
	if _, err := db.ExecContext(ctx,
		`INSERT INTO download_history (id, download_id, title, status, completed_at, media_type, media_id, reason)
		 VALUES ('dh-1', 'd1', 'Release.FLAC', 'acquired', '2024-01-01T10:00:00Z', 'music', ?, '')`, album.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO download_history (id, download_id, title, status, completed_at, media_type, media_id)
		 VALUES ('dh-other', 'd2', 'Movie', 'acquired', '2024-01-03T10:00:00Z', 'movie', ?)`, album.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO import_history (id, media_type, media_id, source_path, dest_path, status, error, imported_at)
		 VALUES ('ih-1', 'music', ?, '/dl/01.flac', '/library/a/01.flac', 'imported', '', '2024-01-02 10:00:00')`, album.ID); err != nil {
		t.Fatal(err)
	}
	history, err := svc.ListArtistHistory(ctx, artist.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].ID != "ih-1" || history[0].Type != "import" ||
		history[1].ID != "dh-1" || history[1].Type != "grab" || history[1].AlbumTitle != album.Title {
		t.Fatalf("history = %+v", history)
	}
	if _, err := svc.ListArtistHistory(ctx, "missing", 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing artist err = %v", err)
	}
}
