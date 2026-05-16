package imports

import (
	"os"
	"path/filepath"
	"strings"
)

// FolderCleaner removes download folders that contain only junk files
// after all media has been imported.
type FolderCleaner struct{}

// junkPatterns lists filenames and glob patterns that should not prevent
// a folder from being cleaned up.
var junkPatterns = []string{
	"Thumbs.db",
	".DS_Store",
	"desktop.ini",
	"*.txt",
	"*.nfo",
	"*.url",
	"*.website",
}

// CleanFolder removes folderPath if it only contains junk files (or is
// empty). Returns true if the folder was removed.
func (c *FolderCleaner) CleanFolder(folderPath string) (bool, error) {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return false, err
	}

	for _, e := range entries {
		if e.IsDir() {
			// Recurse into sub-directories. If the sub-directory can't be
			// cleaned, the parent isn't fully junk either.
			sub := filepath.Join(folderPath, e.Name())
			cleaned, err := c.CleanFolder(sub)
			if err != nil {
				return false, err
			}
			if !cleaned {
				return false, nil
			}
			continue
		}

		if !IsJunk(e.Name()) {
			return false, nil
		}
	}

	// At this point every child was either a junk file or an empty/junk
	// sub-directory that was already removed. Safe to remove the folder.
	if err := os.RemoveAll(folderPath); err != nil {
		return false, err
	}
	return true, nil
}

// IsJunk returns true when filename matches one of the known junk
// patterns (case-insensitive).
func IsJunk(filename string) bool {
	lower := strings.ToLower(filename)
	for _, pat := range junkPatterns {
		patLower := strings.ToLower(pat)
		if matched, _ := filepath.Match(patLower, lower); matched {
			return true
		}
	}
	return false
}
