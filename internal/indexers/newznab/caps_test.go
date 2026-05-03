package newznab

import (
	"os"
	"path/filepath"
	"testing"
)

// loadFixture is a tiny shared helper so each test stays focused on
// the assertion it actually cares about.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func TestParseCapsResponse_FullCaps(t *testing.T) {
	t.Parallel()
	caps, err := parseCapsResponse(loadFixture(t, "caps.xml"))
	if err != nil {
		t.Fatalf("parseCapsResponse: %v", err)
	}

	wantModes := map[string]bool{"search": true, "tvsearch": true, "movie": true, "book": true}
	if len(caps.SearchTypes) != len(wantModes) {
		t.Fatalf("SearchTypes len = %d, want %d (%v)",
			len(caps.SearchTypes), len(wantModes), caps.SearchTypes)
	}
	for _, m := range caps.SearchTypes {
		if !wantModes[m] {
			t.Errorf("unexpected mode %q", m)
		}
	}
	if len(caps.Categories) != 3 {
		t.Errorf("Categories = %v, want 3 entries", caps.Categories)
	}
	wantIDs := map[string]bool{
		"tvdbid": true, "season": true, "ep": true,
		"imdbid": true, "tvmazeid": true, "rid": true,
		"tmdbid": true, "author": true, "title": true,
	}
	if len(caps.SupportedIDs) < 5 {
		t.Errorf("SupportedIDs sparse: %v", caps.SupportedIDs)
	}
	for _, id := range caps.SupportedIDs {
		if !wantIDs[id] {
			t.Errorf("unexpected supported id %q", id)
		}
	}
}

func TestParseCapsResponse_NotXML(t *testing.T) {
	t.Parallel()
	_, err := parseCapsResponse([]byte("<!doctype html><html>nope</html>"))
	if err == nil {
		t.Fatal("expected error on HTML body")
	}
	// HTML body still starts with '<' so it parses; the malformed
	// branch is for things that don't even look like XML:
	_, err = parseCapsResponse([]byte("plain text not even xml"))
	if err == nil {
		t.Fatal("expected error on plain text body")
	}
}

func TestParseCapsResponse_MalformedXML(t *testing.T) {
	t.Parallel()
	_, err := parseCapsResponse([]byte("<caps><searching></caps>"))
	if err == nil {
		t.Fatal("expected parse error on malformed XML")
	}
}
