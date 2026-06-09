package providers

import "testing"

func TestExtractCollectionMBID(t *testing.T) {
	const mbid = "12345678-1234-1234-1234-1234567890ab"
	cases := map[string]string{
		mbid: mbid,
		"https://musicbrainz.org/collection/" + mbid:              mbid,
		"https://musicbrainz.org/collection/" + mbid + "/artists": mbid,
		"https://musicbrainz.org/collection/" + mbid + "?page=2":  mbid,
		"":                 "",
		"  " + mbid + "  ": mbid,
	}
	for in, want := range cases {
		if got := extractCollectionMBID(in); got != want {
			t.Errorf("extractCollectionMBID(%q) = %q, want %q", in, got, want)
		}
	}
}
