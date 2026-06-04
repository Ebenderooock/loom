package cleanup

import (
	"path/filepath"
	"testing"
)

func TestIsWithin(t *testing.T) {
	sep := string(filepath.Separator)
	base := sep + "downloads"
	cases := []struct {
		parent, child string
		want          bool
	}{
		{base, base, true},
		{base, filepath.Join(base, "Movie"), true},
		{base, filepath.Join(base, "tv", "Show", "ep.mkv"), true},
		{filepath.Join(base, "Movie"), filepath.Join(base, "Movie 2"), false}, // sibling, not prefix
		{filepath.Join(base, "Movie"), base, false},                           // child is parent
		{base, sep + "other", false},
		{"", base, false},
		{base, "", false},
	}
	for _, c := range cases {
		if got := isWithin(c.parent, c.child); got != c.want {
			t.Errorf("isWithin(%q,%q)=%v want %v", c.parent, c.child, got, c.want)
		}
	}
}

func TestIsProtected(t *testing.T) {
	sep := string(filepath.Separator)
	dl := sep + "downloads"
	tracked := []string{
		filepath.Join(dl, "tv", "Show S01", "ep01.mkv"), // active download content
		filepath.Join(dl, "Movie (2024)"),               // active download (single dir)
	}

	cases := []struct {
		name  string
		entry string
		want  bool
	}{
		// A category dir that CONTAINS a tracked download must be protected,
		// even though the category dir itself is not a tracked path.
		{"category dir holding tracked content", filepath.Join(dl, "tv"), true},
		// The tracked download dir itself.
		{"tracked movie dir", filepath.Join(dl, "Movie (2024)"), true},
		// A file inside a tracked dir.
		{"file under tracked movie", filepath.Join(dl, "Movie (2024)", "movie.mkv"), true},
		// A genuine orphan: unrelated sibling.
		{"unrelated orphan", filepath.Join(dl, "old-junk"), false},
		// Sibling with shared prefix must NOT be protected.
		{"prefix-sibling not protected", filepath.Join(dl, "Movie (2024) extras"), false},
	}
	for _, c := range cases {
		if got := isProtected(c.entry, tracked); got != c.want {
			t.Errorf("%s: isProtected(%q)=%v want %v", c.name, c.entry, got, c.want)
		}
	}
}

func TestUnderAnyRoot(t *testing.T) {
	sep := string(filepath.Separator)
	roots := []string{sep + "downloads", sep + "data" + sep + "torrents"}
	if !underAnyRoot(filepath.Join(sep+"downloads", "x"), roots) {
		t.Error("expected path under /downloads to be under a root")
	}
	if underAnyRoot(sep+"elsewhere"+sep+"x", roots) {
		t.Error("did not expect /elsewhere/x to be under a root")
	}
}
