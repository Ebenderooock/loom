package customformats

import (
	"os"
	"path/filepath"
	"strings"
)

// ImplFileInfo is the spec type for file-based matching.
const ImplFileInfo = "FileInfoSpec"

// FileInfoSpecification matches against actual file properties after
// download rather than just release name metadata.
type FileInfoSpecification struct {
	Codec      string  `json:"codec,omitempty"`
	Resolution string  `json:"resolution,omitempty"`
	Audio      string  `json:"audio,omitempty"`
	MinSizeGB  float64 `json:"min_size_gb,omitempty"`
	MaxSizeGB  float64 `json:"max_size_gb,omitempty"`
}

// FileMetadata holds real file properties extracted post-download.
type FileMetadata struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	Codec      string `json:"codec"`
	Resolution string `json:"resolution"`
	Audio      string `json:"audio"`
	Extension  string `json:"extension"`
}

// ExtractFileMetadata extracts basic metadata from a file path.
// Full mediainfo parsing is left as a future enhancement; for now we
// derive what we can from the filename and stat.
func ExtractFileMetadata(path string) FileMetadata {
	fm := FileMetadata{
		Path:      path,
		Extension: strings.ToLower(filepath.Ext(path)),
	}

	info, err := os.Stat(path)
	if err == nil {
		fm.Size = info.Size()
	}

	name := strings.ToLower(filepath.Base(path))

	// Resolution detection
	for _, r := range []string{"2160p", "1080p", "720p", "480p"} {
		if strings.Contains(name, r) {
			fm.Resolution = r
			break
		}
	}

	// Codec detection
	for _, c := range []string{"x265", "hevc", "h265", "x264", "h264", "av1"} {
		if strings.Contains(name, c) {
			fm.Codec = c
			break
		}
	}

	// Audio detection
	for _, a := range []string{"atmos", "truehd", "dts-hd", "dts", "aac", "flac", "ac3", "eac3"} {
		if strings.Contains(name, a) {
			fm.Audio = a
			break
		}
	}

	return fm
}

// EvalFile evaluates a custom format against actual file properties.
// Returns true if the format matches the file.
func (e *Engine) EvalFile(cf CustomFormat, filePath string) bool {
	meta := ExtractFileMetadata(filePath)
	ri := ReleaseInfo{
		Title:      filepath.Base(filePath),
		Size:       meta.Size,
		Resolution: meta.Resolution,
		Codec:      meta.Codec,
		Audio:      meta.Audio,
	}
	return matchesFormat(cf, ri)
}

// ScoreFile evaluates all loaded custom formats against a real file
// and returns every match. This is the post-import counterpart of
// ScoreRelease.
func (e *Engine) ScoreFile(filePath string) []FormatMatch {
	var matches []FormatMatch
	for _, cf := range e.formats {
		if e.EvalFile(cf, filePath) {
			matches = append(matches, FormatMatch{
				CustomFormatID:   cf.ID,
				CustomFormatName: cf.Name,
			})
		}
	}
	return matches
}
