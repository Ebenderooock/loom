package musicbrainz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestArtistCatalogOfficialTypesAndPagination(t *testing.T) {
	group := func(id, primary string, secondary ...string) *ReleaseGroupResponse {
		return &ReleaseGroupResponse{
			ID: id, Title: id, PrimaryType: primary, SecondaryTypes: secondary,
			FirstReleaseDate: "2000-10-24",
		}
	}
	pages := map[string][]ReleaseResponse{
		"0": {
			{Status: "Official", ReleaseGroup: group("studio", "Album")},
			{Status: "Official", ReleaseGroup: group("studio", "Album")},
		},
		"2": {
			{Status: "Official", ReleaseGroup: group("studio", "Album")},
			{Status: "Official", ReleaseGroup: group("ep", "EP")},
			{Status: "Official", ReleaseGroup: group("single", "Single")},
			{Status: "Official", ReleaseGroup: group("live", "Album", "Live")},
			{Status: "Official", ReleaseGroup: group("compilation", "Album", "Compilation")},
			{Status: "Official", ReleaseGroup: group("remix", "Album", "Remix")},
			{Status: "Official", ReleaseGroup: group("live-single", "Single", "Live")},
			{Status: "Official", ReleaseGroup: group("broadcast", "Broadcast")},
			{Status: "Official", ReleaseGroup: group("other", "Other")},
			{Status: "Bootleg", ReleaseGroup: group("bootleg", "Album")},
			{Status: "Promotion", ReleaseGroup: group("promo", "Single")},
			{Status: "Pseudo-Release", ReleaseGroup: group("pseudo", "Album")},
			{Status: "", ReleaseGroup: group("unknown", "Album")},
			{Status: "Official"},
			{Status: "Official", ReleaseGroup: group("", "Album")},
			{Status: "Official", ReleaseGroup: group("untyped", "")},
		},
	}
	var offsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/release" || q.Get("artist") != "linkin-park" ||
			q.Get("status") != "official" || q.Get("type") != "album|ep|single" ||
			q.Get("inc") != "release-groups" || q.Get("limit") != "100" {
			t.Errorf("unexpected catalogue request: %s", r.URL)
		}
		offset := q.Get("offset")
		offsets = append(offsets, offset)
		// MusicBrainz can return fewer releases than the requested page size.
		page, ok := pages[offset]
		if !ok {
			t.Errorf("unexpected page offset %q", offset)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(releaseBrowseResponse{Count: 18, Releases: page})
	}))
	defer server.Close()
	client := NewClient(&Config{BaseURL: server.URL})
	client.throttler = NewThrottler(0)
	albums, err := NewProvider(client).GetArtistAlbums(context.Background(), "linkin-park")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(albums))
	for _, album := range albums {
		ids = append(ids, album.MBID)
		if album.ReleaseDate != "2000-10-24" || album.CoverArtURL == "" {
			t.Fatalf("release-group metadata lost: %+v", album)
		}
	}
	if !reflect.DeepEqual(ids, []string{"studio", "ep", "single"}) {
		t.Fatalf("catalogue = %v, want studio, ep, single exactly once", ids)
	}
	if !reflect.DeepEqual(offsets, []string{"0", "2"}) {
		t.Fatalf("page offsets = %v, want [0 2]", offsets)
	}
}

func TestArtistCatalogDoesNotReturnPartialResultsOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "0" {
			http.Error(w, "unavailable", http.StatusBadRequest)
			return
		}

		_ = json.NewEncoder(w).Encode(releaseBrowseResponse{
			Count: 2,
			Releases: []ReleaseResponse{{
				Status: "Official",
				ReleaseGroup: &ReleaseGroupResponse{
					ID: "studio", PrimaryType: "Album",
				},
			}},
		})
	}))
	defer server.Close()
	client := NewClient(&Config{BaseURL: server.URL})
	client.throttler = NewThrottler(0)
	albums, err := NewProvider(client).GetArtistAlbums(context.Background(), "artist")
	if err == nil || albums != nil {
		t.Fatalf("partial catalogue returned: albums=%v, error=%v", albums, err)
	}
}

func TestArtistCatalogRejectsTruncatedBrowse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(releaseBrowseResponse{Count: 1})
	}))
	defer server.Close()
	client := NewClient(&Config{BaseURL: server.URL})
	albums, err := NewProvider(client).GetArtistAlbums(context.Background(), "artist")
	if err == nil || albums != nil {
		t.Fatalf("truncated catalogue accepted: albums=%v, error=%v", albums, err)
	}
}
