package safety

import (
	"os"
	"path/filepath"
	"strings"
)

// expectedVideoExts are the file types we expect to find after download.
var expectedVideoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".m4v": true,
}

// PostValidationResult captures the outcome of a post-download file scan.
type PostValidationResult struct {
	Pass    bool     `json:"pass"`
	Reasons []string `json:"reasons"`
}

// PostValidator scans downloaded files for integrity issues.
type PostValidator struct {
	cfg ReleaseValidatorConfig
}

// NewPostValidator creates a post-download validator with the given config.
func NewPostValidator(cfg ReleaseValidatorConfig) *PostValidator {
	return &PostValidator{cfg: cfg}
}

// ValidateDownload inspects the files at downloadPath after a download
// completes and returns whether the release looks legitimate.
func (v *PostValidator) ValidateDownload(downloadPath string) (PostValidationResult, error) {
	var reasons []string
	hasVideo := false
	hasExecutable := false

	err := filepath.Walk(downloadPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))

		if expectedVideoExts[ext] {
			hasVideo = true
		}

		if dangerousExtensions[ext] {
			hasExecutable = true
			reasons = append(reasons, "unexpected executable found: "+filepath.Base(path))
		}

		// Detect password-protected RAR archives by extension heuristic.
		// A proper check would inspect the RAR header, but the filename
		// pattern "password" is a strong signal.
		if ext == ".rar" {
			lower := strings.ToLower(filepath.Base(path))
			if strings.Contains(lower, "password") {
				reasons = append(reasons, "possibly password-protected archive: "+filepath.Base(path))
			}
		}

		return nil
	})
	if err != nil {
		return PostValidationResult{Pass: false, Reasons: []string{"failed to scan download path: " + err.Error()}}, err
	}

	if !hasVideo {
		reasons = append(reasons, "no expected video files found (.mkv, .mp4, .avi, .m4v)")
	}

	if hasExecutable {
		reasons = append(reasons, "executables should not be present alongside media files")
	}

	// Size sanity: check total size of video files.
	if hasVideo {
		var totalBytes int64
		_ = filepath.Walk(downloadPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if expectedVideoExts[ext] {
				totalBytes += info.Size()
			}
			return nil
		})
		totalMB := totalBytes / (1024 * 1024)
		if v.cfg.MinMovieSizeMB > 0 && totalMB < v.cfg.MinMovieSizeMB {
			reasons = append(reasons, "video files are suspiciously small")
		}
	}

	if len(reasons) == 0 {
		return PostValidationResult{Pass: true}, nil
	}
	return PostValidationResult{Pass: false, Reasons: reasons}, nil
}
