package imports

import (
	"os"
	"path/filepath"
	"strings"
)

// ExtraService handles importing NFO and image files alongside video files.
type ExtraService struct{}

// extraExtensions lists recognized extra file extensions.
var extraExtensions = map[string]bool{
	".nfo": true, ".jpg": true, ".jpeg": true, ".png": true, ".tbn": true,
}

// FindExtras scans the directory of videoPath and returns paths to extra
// files whose names start with the video's stem.
func (e *ExtraService) FindExtras(videoPath string) ([]string, error) {
	dir := filepath.Dir(videoPath)
	videoStem := filenameStem(videoPath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var extras []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !extraExtensions[ext] {
			continue
		}
		stem := filenameStem(filepath.Join(dir, name))
		if !strings.HasPrefix(stem, videoStem) {
			continue
		}
		extras = append(extras, filepath.Join(dir, name))
	}
	return extras, nil
}

// ImportExtra copies/moves an extra file to sit beside destVideoPath,
// preserving the original extension.
func (e *ExtraService) ImportExtra(extraPath string, destVideoPath string, mode ImportMode) error {
	destDir := filepath.Dir(destVideoPath)
	videoStem := filenameStem(destVideoPath)
	ext := strings.ToLower(filepath.Ext(extraPath))

	dst := filepath.Join(destDir, videoStem+ext)
	return importFile(extraPath, dst, mode)
}
