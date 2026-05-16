package imports

import (
	"os"
	"path/filepath"
	"strings"
)

// SubtitleService handles importing subtitle files alongside video files.
type SubtitleService struct{}

// subtitleExtensions lists recognized subtitle file extensions.
var subtitleExtensions = map[string]bool{
	".srt": true, ".ass": true, ".ssa": true, ".sub": true,
	".idx": true, ".sup": true, ".vtt": true, ".jss": true,
	".psb": true, ".rt": true, ".smi": true, ".txt": true, ".utf": true,
}

// SubtitleFile describes a subtitle found alongside a video.
type SubtitleFile struct {
	Path     string // full source path
	Language string // extracted language code (e.g. "en", "eng") or empty
	IsForced bool   // filename contains ".forced."
	IsSDH    bool   // filename contains ".sdh." or ".cc."
}

// FindSubtitles scans the directory of videoPath and returns subtitle
// files whose names start with the video's stem (filename without ext).
func (s *SubtitleService) FindSubtitles(videoPath string) ([]SubtitleFile, error) {
	dir := filepath.Dir(videoPath)
	videoStem := filenameStem(videoPath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var subs []SubtitleFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !subtitleExtensions[ext] {
			continue
		}
		stem := filenameStem(filepath.Join(dir, name))
		if !strings.HasPrefix(stem, videoStem) {
			continue
		}

		suffix := stem[len(videoStem):] // e.g. ".en.forced"
		subs = append(subs, SubtitleFile{
			Path:     filepath.Join(dir, name),
			Language: extractLanguage(suffix),
			IsForced: containsFold(suffix, ".forced"),
			IsSDH:    containsFold(suffix, ".sdh") || containsFold(suffix, ".cc"),
		})
	}
	return subs, nil
}

// ImportSubtitle copies/moves a subtitle to sit beside destVideoPath.
func (s *SubtitleService) ImportSubtitle(sub SubtitleFile, destVideoPath string, mode ImportMode) error {
	destDir := filepath.Dir(destVideoPath)
	videoStem := filenameStem(destVideoPath)
	ext := strings.ToLower(filepath.Ext(sub.Path))

	name := videoStem
	if sub.Language != "" {
		name += "." + sub.Language
	}
	if sub.IsForced {
		name += ".forced"
	}
	if sub.IsSDH {
		name += ".sdh"
	}
	name += ext

	dst := filepath.Join(destDir, name)
	return importFile(sub.Path, dst, mode)
}

// filenameStem returns the filename without its final extension.
func filenameStem(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// extractLanguage pulls a language code from a subtitle suffix like
// ".en", ".eng", ".English", ".en.forced".
func extractLanguage(suffix string) string {
	parts := strings.Split(suffix, ".")
	for _, p := range parts {
		if p == "" {
			continue
		}
		low := strings.ToLower(p)
		if low == "forced" || low == "sdh" || low == "cc" {
			continue
		}
		// Accept 2-3 letter codes or full language names
		if len(p) >= 2 {
			return p
		}
	}
	return ""
}

// containsFold reports whether s contains substr, case-insensitively.
func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
