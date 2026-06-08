package music

import (
	"fmt"
	"path/filepath"
	"strings"
)

// BuildTrackPath returns the canonical relative path for a track file within a
// music library, following the layout:
//
//	{Artist}/{Album} ({Year})/{disc-}{track:00} - {Title}{ext}
//
// The disc prefix is only included when the album spans multiple discs. The
// extension should include the leading dot (e.g. ".flac"); when empty no
// extension is appended.
func BuildTrackPath(artist *Artist, album *Album, track *Track, multiDisc bool, ext string) string {
	artistDir := sanitizeComponent(artistName(artist))
	albumDir := sanitizeComponent(albumFolderName(album))
	fileName := trackFileName(track, multiDisc, ext)
	return filepath.Join(artistDir, albumDir, fileName)
}

func artistName(a *Artist) string {
	if a == nil || strings.TrimSpace(a.Name) == "" {
		return "Unknown Artist"
	}
	return a.Name
}

func albumFolderName(al *Album) string {
	if al == nil || strings.TrimSpace(al.Title) == "" {
		return "Unknown Album"
	}
	if year := albumYear(al); year != "" {
		return fmt.Sprintf("%s (%s)", al.Title, year)
	}
	return al.Title
}

// albumYear extracts a 4-digit year from the album release date (YYYY-MM-DD or YYYY).
func albumYear(al *Album) string {
	if al == nil {
		return ""
	}
	d := strings.TrimSpace(al.ReleaseDate)
	if len(d) >= 4 {
		y := d[:4]
		if isDigits(y) {
			return y
		}
	}
	return ""
}

func trackFileName(t *Track, multiDisc bool, ext string) string {
	title := "Unknown Track"
	num := 0
	disc := 1
	if t != nil {
		if strings.TrimSpace(t.Title) != "" {
			title = t.Title
		}
		num = t.TrackNumber
		if t.DiscNumber > 0 {
			disc = t.DiscNumber
		}
	}
	var prefix string
	if multiDisc {
		prefix = fmt.Sprintf("%d-%02d", disc, num)
	} else {
		prefix = fmt.Sprintf("%02d", num)
	}
	name := fmt.Sprintf("%s - %s", prefix, sanitizeComponent(title))
	if ext != "" {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		name += ext
	}
	return name
}

// sanitizeComponent strips characters that are illegal in path components on
// common filesystems and trims problematic leading/trailing characters.
func sanitizeComponent(s string) string {
	s = strings.TrimSpace(s)
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", " -",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
		"\x00", "",
	)
	s = replacer.Replace(s)
	s = strings.Trim(s, " .")
	if s == "" {
		return "Unknown"
	}
	return s
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
