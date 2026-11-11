package artwork

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testMBID = "f59c5520-5f46-4d2c-b2c4-822eabf53419"

func newTestServer(t *testing.T, fanartStatus int, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch {
		case strings.HasPrefix(r.URL.Path, "/fanart/music/"):
			if r.Header.Get("api-key") != "fan-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if fanartStatus != http.StatusOK {
				w.WriteHeader(fanartStatus)
				return
			}
			_, _ = w.Write([]byte(`{"artistthumb":[{"url":"https://fan/thumb.jpg"}],
				"artistbackground":[{"url":"https://fan/bg.jpg"}],
				"musiclogo":[{"url":"https://fan/logo.png"}],
				"albums":{"rg-1":{"albumcover":[{"url":"https://fan/rg1.jpg"}]},"rg-2":{"albumcover":[]}}}`))
		case strings.HasSuffix(r.URL.Path, "/artist-mb.php"):
			_, _ = w.Write([]byte(`{"artists":[{"strArtistThumb":"https://db/thumb.jpg","strArtistFanart":"https://db/bg.jpg",
				"strArtistLogo":"https://db/logo.png","strBiographyEN":null,"strBiography":" A band. "}]}`))
		case strings.HasSuffix(r.URL.Path, "/album-mb.php"):
			if r.URL.Query().Get("i") == "missing" {
				_, _ = w.Write([]byte(`{"album":null}`))
				return
			}
			_, _ = w.Write([]byte(`{"album":[{"strAlbumThumb":"https://db/album.jpg","strDescriptionEN":"An album."}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newTestProvider(srv *httptest.Server, fanartKey string) *Provider {
	return New(Config{
		FanartAPIKey:   fanartKey,
		FanartBaseURL:  srv.URL + "/fanart",
		AudioDBBaseURL: srv.URL + "/db",
	})
}

func TestArtistArtworkPrefersFanartAndFallsBackToAudioDB(t *testing.T) {
	var calls atomic.Int32
	srv := newTestServer(t, http.StatusOK, &calls)
	defer srv.Close()
	p := newTestProvider(srv, "fan-key")

	got, err := p.ArtistArtwork(context.Background(), testMBID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ThumbURL != "https://fan/thumb.jpg" || got.BackgroundURL != "https://fan/bg.jpg" || got.LogoURL != "https://fan/logo.png" {
		t.Fatalf("fanart images not preferred: %+v", got)
	}
	if got.Biography != "A band." {
		t.Fatalf("biography = %q", got.Biography)
	}
	if got.AlbumCovers["rg-1"] != "https://fan/rg1.jpg" || len(got.AlbumCovers) != 1 {
		t.Fatalf("album covers = %v", got.AlbumCovers)
	}

	before := calls.Load()
	if _, err := p.ArtistArtwork(context.Background(), testMBID); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != before {
		t.Fatal("second lookup should be served from cache")
	}
}

func TestArtistArtworkWithoutFanartKeyUsesAudioDB(t *testing.T) {
	var calls atomic.Int32
	srv := newTestServer(t, http.StatusOK, &calls)
	defer srv.Close()
	p := newTestProvider(srv, "")

	got, err := p.ArtistArtwork(context.Background(), testMBID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ThumbURL != "https://db/thumb.jpg" || got.BackgroundURL != "https://db/bg.jpg" {
		t.Fatalf("audiodb images not used: %+v", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected only the TheAudioDB call, got %d", calls.Load())
	}
}

func TestArtistArtworkFanartFailureIsPartial(t *testing.T) {
	var calls atomic.Int32
	srv := newTestServer(t, http.StatusInternalServerError, &calls)
	defer srv.Close()
	p := newTestProvider(srv, "fan-key")

	got, err := p.ArtistArtwork(context.Background(), testMBID)
	if err != nil {
		t.Fatalf("partial failure should not error: %v", err)
	}
	if got.ThumbURL != "https://db/thumb.jpg" {
		t.Fatalf("expected TheAudioDB fallback, got %+v", got)
	}
}

func TestAlbumArtwork(t *testing.T) {
	var calls atomic.Int32
	srv := newTestServer(t, http.StatusOK, &calls)
	defer srv.Close()
	p := newTestProvider(srv, "")

	got, err := p.AlbumArtwork(context.Background(), "rg-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CoverURL != "https://db/album.jpg" || got.Description != "An album." {
		t.Fatalf("album artwork = %+v", got)
	}
	missing, err := p.AlbumArtwork(context.Background(), "missing")
	if err != nil || missing.CoverURL != "" {
		t.Fatalf("missing album = %+v, %v", missing, err)
	}
}

func TestTestKeysReportsRejectedFanartKey(t *testing.T) {
	var calls atomic.Int32
	srv := newTestServer(t, http.StatusOK, &calls)
	defer srv.Close()
	p := newTestProvider(srv, "")

	fanErr, dbErr := p.TestKeys(context.Background(), "wrong", "456")
	if fanErr == nil || !strings.Contains(fanErr.Error(), "rejected") {
		t.Fatalf("fanart error = %v", fanErr)
	}
	if dbErr != nil {
		t.Fatalf("audiodb error = %v", dbErr)
	}
}

func TestTransportErrorsDoNotLeakKeys(t *testing.T) {
	p := New(Config{AudioDBAPIKey: "secret-key", AudioDBBaseURL: "http://127.0.0.1:1/db"})
	_, err := p.ArtistArtwork(context.Background(), testMBID)
	if err == nil || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("error should not contain the key: %v", err)
	}
}
