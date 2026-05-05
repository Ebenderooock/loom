package imports

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ConflictPolicy controls how destination conflicts are resolved.
type ConflictPolicy string

const (
	ConflictSkip            ConflictPolicy = "skip"
	ConflictReplaceIfBetter ConflictPolicy = "replace_if_better"
	ConflictReplaceAlways   ConflictPolicy = "replace_always"
	ConflictKeepBoth        ConflictPolicy = "keep_both"
)

// ConflictAction is the resolved action for a conflict.
type ConflictAction string

const (
	ActionSkip    ConflictAction = "skip"
	ActionReplace ConflictAction = "replace"
	ActionKeep    ConflictAction = "keep_both"
	ActionImport  ConflictAction = "import" // no conflict
)

// FileQuality holds parsed quality metadata from a filename.
type FileQuality struct {
	Resolution int    // e.g. 2160, 1080, 720
	Source     string // e.g. "bluray", "webrip", "hdtv"
	Codec      string // e.g. "x265", "x264"
}

// FileInfo describes a file for conflict comparison.
type FileInfo struct {
	Path    string      `json:"path"`
	Size    int64       `json:"size"`
	Quality FileQuality `json:"quality"`
}

// ConflictDecision records the outcome of a conflict resolution.
type ConflictDecision struct {
	Action ConflictAction `json:"action"`
	Reason string         `json:"reason"`
}

// ParseFileQuality extracts quality indicators from a filename.
func ParseFileQuality(filename string) FileQuality {
	name := strings.ToLower(filepath.Base(filename))
	var q FileQuality

	// Resolution
	if strings.Contains(name, "2160p") || strings.Contains(name, "4k") {
		q.Resolution = 2160
	} else if strings.Contains(name, "1080p") {
		q.Resolution = 1080
	} else if strings.Contains(name, "720p") {
		q.Resolution = 720
	} else if strings.Contains(name, "480p") {
		q.Resolution = 480
	}

	// Source
	for _, src := range []string{"remux", "bluray", "bdrip", "web-dl", "webdl", "webrip", "hdtv", "dvdrip"} {
		if strings.Contains(name, src) {
			q.Source = src
			break
		}
	}

	// Codec
	for _, c := range []string{"x265", "hevc", "h265", "x264", "h264"} {
		if strings.Contains(name, c) {
			q.Codec = c
			break
		}
	}

	return q
}

// qualityScore returns a numeric score for quality comparison. Higher is better.
func qualityScore(q FileQuality) int {
	score := 0

	// Resolution weight
	switch q.Resolution {
	case 2160:
		score += 400
	case 1080:
		score += 300
	case 720:
		score += 200
	case 480:
		score += 100
	}

	// Source weight
	switch q.Source {
	case "remux":
		score += 50
	case "bluray", "bdrip":
		score += 40
	case "web-dl", "webdl":
		score += 30
	case "webrip":
		score += 25
	case "hdtv":
		score += 20
	case "dvdrip":
		score += 10
	}

	// Codec weight (h265/x265 is more efficient)
	switch q.Codec {
	case "x265", "hevc", "h265":
		score += 5
	case "x264", "h264":
		score += 3
	}

	return score
}

// ResolveConflict determines the action when a destination file already exists.
func ResolveConflict(existing, incoming FileInfo, policy ConflictPolicy) ConflictDecision {
	switch policy {
	case ConflictSkip:
		return ConflictDecision{
			Action: ActionSkip,
			Reason: fmt.Sprintf("policy is skip; existing file %s kept", filepath.Base(existing.Path)),
		}

	case ConflictReplaceAlways:
		return ConflictDecision{
			Action: ActionReplace,
			Reason: "policy is replace_always",
		}

	case ConflictKeepBoth:
		return ConflictDecision{
			Action: ActionKeep,
			Reason: "policy is keep_both; both files will be retained",
		}

	case ConflictReplaceIfBetter:
		existingScore := qualityScore(existing.Quality)
		incomingScore := qualityScore(incoming.Quality)

		if incomingScore > existingScore {
			return ConflictDecision{
				Action: ActionReplace,
				Reason: fmt.Sprintf("incoming quality score %d > existing %d (resolution: %dp→%dp)",
					incomingScore, existingScore, existing.Quality.Resolution, incoming.Quality.Resolution),
			}
		}
		if incomingScore == existingScore && incoming.Size > existing.Size {
			return ConflictDecision{
				Action: ActionReplace,
				Reason: fmt.Sprintf("same quality score %d but incoming size %d > existing %d",
					incomingScore, incoming.Size, existing.Size),
			}
		}
		return ConflictDecision{
			Action: ActionSkip,
			Reason: fmt.Sprintf("existing quality score %d >= incoming %d; keeping existing",
				existingScore, incomingScore),
		}

	default:
		return ConflictDecision{
			Action: ActionSkip,
			Reason: fmt.Sprintf("unknown policy %q; defaulting to skip", policy),
		}
	}
}

// NewFileInfo creates a FileInfo by stat-ing the path and parsing quality.
func NewFileInfo(path string) (FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{
		Path:    path,
		Size:    info.Size(),
		Quality: ParseFileQuality(path),
	}, nil
}

// versionedPath returns a path with a version suffix to avoid overwriting.
// e.g. /movies/Movie/movie.mkv → /movies/Movie/movie (2).mkv
func versionedPath(dest string) string {
	dir := filepath.Dir(dest)
	ext := filepath.Ext(dest)
	base := strings.TrimSuffix(filepath.Base(dest), ext)

	// Check if already versioned
	re := regexp.MustCompile(`\s*\((\d+)\)$`)
	startN := 2
	if m := re.FindStringSubmatch(base); len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		startN = n + 1
		base = strings.TrimSpace(re.ReplaceAllString(base, ""))
	}

	for n := startN; n < 100; n++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, n, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	// Fallback: just append a large number
	return filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, 99, ext))
}
