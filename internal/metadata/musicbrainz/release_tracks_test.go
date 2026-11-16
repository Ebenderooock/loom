package musicbrainz

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAlbumReleaseDecodesNumericPositions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// MusicBrainz positions are numbers; only the printed track number
		// is a string (and can contain vinyl labels such as "A1").
		_, _ = w.Write([]byte(`{
			"id":"release-id","title":"From Zero",
			"media":[
				{"position":1,"format":"Digital Media","tracks":[
					{"id":"track-1","position":1,"number":"1","title":"From Zero (Intro)",
					 "length":22000,"recording":{"id":"recording-1","title":"From Zero (Intro)"}},
					{"id":"track-2","position":2,"number":"2","title":"The Emptiness Machine",
					 "length":190000}
				]},
				{"position":2,"tracks":[
					{"id":"track-3","position":3,"number":"A3","title":"Bonus track"},
					{"id":"track-4","position":4,"title":"Position fallback"}
				]},
				{"tracks":[{"id":"track-5","title":"Index fallback"}]}
			]
		}`))
	}))
	defer server.Close()
	provider := NewProvider(NewClient(&Config{BaseURL: server.URL}))
	release, err := provider.GetAlbumRelease(t.Context(), "release-id")
	if err != nil {
		t.Fatal(err)
	}
	if release.TrackCount != 5 || len(release.Tracks) != 5 {
		t.Fatalf("tracks were not decoded: %+v", release)
	}
	for i, want := range [][2]int{{1, 1}, {1, 2}, {2, 3}, {2, 4}, {3, 1}} {
		track := release.Tracks[i]
		if track.DiscNumber != want[0] || track.TrackNumber != want[1] {
			t.Errorf("track %d position = %d/%d, want %d/%d", i,
				track.DiscNumber, track.TrackNumber, want[0], want[1])
		}
	}
	if release.Tracks[0].MBID != "recording-1" || release.Tracks[0].DurationMs != 22000 {
		t.Fatalf("recording identity or duration lost: %+v", release.Tracks[0])
	}
}

func TestInvalidReleaseResponseIsNotANetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"media":[{"position":"invalid"}]}`))
	}))
	defer server.Close()
	_, err := NewClient(&Config{BaseURL: server.URL}).GetReleaseRaw(t.Context(), "release-id")
	var clientErr *ClientError
	var decodeErr *json.UnmarshalTypeError
	if !errors.As(err, &clientErr) || clientErr.Code != ErrCodeResponseError ||
		clientErr.StatusCode != http.StatusOK || !errors.As(err, &decodeErr) {
		t.Fatalf("JSON failure misclassified or cause lost: %v", err)
	}
}
