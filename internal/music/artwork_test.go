package music

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/storage"
)

type fakeArtwork struct {
	artist    *metadata.ArtistArtwork
	album     *metadata.AlbumArtwork
	artistErr error
}

func (f *fakeArtwork) ArtistArtwork(context.Context, string) (*metadata.ArtistArtwork, error) {
	return f.artist, f.artistErr
}

func (f *fakeArtwork) AlbumArtwork(context.Context, string) (*metadata.AlbumArtwork, error) {
	return f.album, nil
}

func TestAddArtistAppliesArtworkAndDetails(t *testing.T) {
	ctx := context.Background()
	prov := sampleProvider()
	prov.artist.ImageURL = "https://commons.example/image.jpg"
	prov.artist.BeginArea = "Agoura Hills"
	prov.artist.BeginDate = "1996"
	prov.artist.Rating = 4.5
	prov.artist.RatingVotes = 12
	prov.artist.Links = []metadata.ExternalLink{{Type: "spotify", URL: "https://open.spotify.com/artist/x"}}
	svc, _ := newTestService(t, prov)
	svc.(*service).SetArtwork(&fakeArtwork{artist: &metadata.ArtistArtwork{
		ThumbURL:      "https://fanart.example/thumb.jpg",
		BackgroundURL: "https://fanart.example/bg.jpg",
		LogoURL:       "https://fanart.example/logo.png",
		Biography:     "A band.",
		AlbumCovers:   map[string]string{"rg-1": "https://fanart.example/rg1.jpg"},
	}})

	artist, err := svc.AddArtist(ctx, AddArtistRequest{MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1"})
	if err != nil {
		t.Fatalf("AddArtist: %v", err)
	}
	if artist.ImageURL != "https://fanart.example/thumb.jpg" || artist.BackgroundURL == "" || artist.LogoURL == "" {
		t.Fatalf("artwork not applied: %+v", artist)
	}
	if artist.Overview != "A band." || artist.BeginArea != "Agoura Hills" || artist.Rating != 4.5 || artist.RatingVotes != 12 {
		t.Fatalf("details not applied: %+v", artist)
	}
	if len(artist.Links) != 1 || artist.Links[0].Type != "spotify" {
		t.Fatalf("links not stored: %+v", artist.Links)
	}
	for _, al := range artist.Albums {
		if al.MBID == "rg-1" && al.CoverArtURL != "https://fanart.example/rg1.jpg" {
			t.Errorf("rg-1 cover = %q, want fanart cover", al.CoverArtURL)
		}
	}
}

func TestAddArtistFallsBackToCommonsImageWhenArtworkFails(t *testing.T) {
	ctx := context.Background()
	prov := sampleProvider()
	prov.artist.ImageURL = "https://commons.example/image.jpg"
	svc, _ := newTestService(t, prov)
	svc.(*service).SetArtwork(&fakeArtwork{artistErr: errors.New("down")})

	artist, err := svc.AddArtist(ctx, AddArtistRequest{MBID: "artist-1", QualityProfileID: "qp-1", LibraryID: "lib-1"})
	if err != nil {
		t.Fatalf("AddArtist must not fail on artwork errors: %v", err)
	}
	if artist.ImageURL != "https://commons.example/image.jpg" {
		t.Fatalf("image = %q, want Commons fallback", artist.ImageURL)
	}
}

func TestGetAlbumFetchesDetailsOnce(t *testing.T) {
	ctx := context.Background()
	prov := sampleProvider()
	prov.albums[0].Rating = 3.5
	prov.albums[0].Links = []metadata.ExternalLink{{Type: "wikidata", URL: "https://www.wikidata.org/wiki/Q1"}}
	svc, repo := newTestService(t, prov)
	svc.(*service).SetArtwork(&fakeArtwork{album: &metadata.AlbumArtwork{
		CoverURL: "https://tadb.example/cover.jpg", Description: "Debut album.",
	}})

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
	album, err := svc.GetAlbum(ctx, albumID)
	if err != nil {
		t.Fatalf("GetAlbum: %v", err)
	}
	if album.DetailsFetchedAt == nil || album.Overview != "Debut album." || album.Rating != 3.5 {
		t.Fatalf("details not applied: %+v", album)
	}
	if album.CoverArtURL != "https://tadb.example/cover.jpg" || len(album.Links) != 1 {
		t.Fatalf("cover/links not applied: cover=%q links=%+v", album.CoverArtURL, album.Links)
	}
	stored, _ := repo.GetAlbum(ctx, albumID)
	if stored == nil || stored.DetailsFetchedAt == nil {
		t.Fatal("details timestamp not persisted")
	}
}

type recordingKeyApplier struct {
	fanart, audioDB string
	fanartErr       error
}

func (a *recordingKeyApplier) SetKeys(f, d string) { a.fanart, a.audioDB = f, d }

func (a *recordingKeyApplier) TestKeys(_ context.Context, f, _ string) (error, error) {
	if f == "" {
		return nil, nil
	}
	return a.fanartErr, nil
}

func newArtworkSettingsTestService(t *testing.T, envFanart string) (*ArtworkSettingsService, *recordingKeyApplier) {
	t.Helper()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "artwork.db")},
	}
	db, err := storage.Open(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	applier := &recordingKeyApplier{}
	return NewArtworkSettingsService(NewArtworkSettingsStore(db.DB()), applier, envFanart, ""), applier
}

func TestArtworkSettingsStoredKeysOverrideEnvAndCanBeCleared(t *testing.T) {
	ctx := context.Background()
	svc, applier := newArtworkSettingsTestService(t, "env-key")

	got, err := svc.ApplyStored(ctx)
	if err != nil {
		t.Fatalf("ApplyStored: %v", err)
	}
	if applier.fanart != "env-key" || got.FanartKeySet || !got.FanartKeyFromEnv {
		t.Fatalf("env fallback not applied: applier=%q settings=%+v", applier.fanart, got)
	}

	got, err = svc.Update(ctx, ArtworkSettingsUpdate{FanartAPIKey: " stored-key ", AudioDBAPIKey: "tadb"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if applier.fanart != "stored-key" || applier.audioDB != "tadb" || !got.FanartKeySet || got.FanartKeyFromEnv {
		t.Fatalf("stored keys not applied: applier=%+v settings=%+v", applier, got)
	}

	// An empty key keeps the stored value.
	if _, err := svc.Update(ctx, ArtworkSettingsUpdate{}); err != nil {
		t.Fatalf("Update keep: %v", err)
	}
	if applier.fanart != "stored-key" {
		t.Fatalf("empty update dropped key: %q", applier.fanart)
	}

	got, err = svc.Update(ctx, ArtworkSettingsUpdate{ClearFanartAPIKey: true})
	if err != nil {
		t.Fatalf("Update clear: %v", err)
	}
	if applier.fanart != "env-key" || got.FanartKeySet {
		t.Fatalf("clear did not fall back to env: applier=%q settings=%+v", applier.fanart, got)
	}

	if _, err := svc.Update(ctx, ArtworkSettingsUpdate{FanartAPIKey: "bad key"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for key with space, got %v", err)
	}
}

func TestArtworkSettingsRouterNeverReturnsKeys(t *testing.T) {
	svc, applier := newArtworkSettingsTestService(t, "")
	applier.fanartErr = errors.New("fanart.tv: API key rejected")
	h := ArtworkSettingsRouter(svc)

	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"fanart_api_key":"secret-value"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-value") {
		t.Fatalf("response leaked key: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var res ArtworkTestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.OK || !res.FanartTested || res.FanartOK || res.FanartError == "" {
		t.Fatalf("expected stored fanart key to be tested and fail: %+v", res)
	}

	req = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"unknown":1}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", rec.Code)
	}
}
