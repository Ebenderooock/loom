package imports

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ScanResult describes a potential match found during a folder scan.
type ScanResult struct {
	FilePath        string  `json:"file_path"`
	FileSize        int64   `json:"file_size"`
	DetectedTitle   string  `json:"detected_title"`
	DetectedYear    int     `json:"detected_year,omitempty"`
	DetectedSeason  int     `json:"detected_season,omitempty"`
	DetectedEpisode int     `json:"detected_episode,omitempty"`
	MatchedMedia    string  `json:"matched_media,omitempty"`
	MatchedMediaID  string  `json:"matched_media_id,omitempty"`
	MediaType       string  `json:"media_type,omitempty"`
	Confidence      float64 `json:"confidence"`
	SuggestedAction string  `json:"suggested_action"`
	Quality         string  `json:"quality,omitempty"`
}

// ScanFolder scans a directory for media files and returns potential matches
// without actually importing anything. Useful for previewing before import.
func (p *ImportPipeline) ScanFolder(ctx context.Context, path string) ([]ScanResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}

	var mediaFiles []string
	if info.IsDir() {
		mediaFiles, err = scanMediaFiles(path)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
	} else {
		ext := filepath.Ext(path)
		if !mediaExtensions[ext] {
			return nil, fmt.Errorf("not a media file: %s", path)
		}
		mediaFiles = []string{path}
	}

	var results []ScanResult
	for _, mf := range mediaFiles {
		result := p.scanSingleFile(ctx, mf)
		results = append(results, result)
	}

	return results, nil
}

// scanSingleFile analyses a single file and attempts to match it.
func (p *ImportPipeline) scanSingleFile(ctx context.Context, filePath string) ScanResult {
	info, _ := os.Stat(filePath)
	var fileSize int64
	if info != nil {
		fileSize = info.Size()
	}

	parsed := parseReleaseName(filepath.Base(filePath))
	quality := ParseFileQuality(filePath)
	qualityStr := ""
	if quality.Resolution > 0 {
		qualityStr = fmt.Sprintf("%dp", quality.Resolution)
	}
	if quality.Source != "" {
		if qualityStr != "" {
			qualityStr += "/"
		}
		qualityStr += quality.Source
	}

	result := ScanResult{
		FilePath:        filePath,
		FileSize:        fileSize,
		DetectedTitle:   parsed.Title,
		DetectedYear:    parsed.Year,
		DetectedSeason:  parsed.Season,
		DetectedEpisode: parsed.Episode,
		Quality:         qualityStr,
		SuggestedAction: "skip",
		Confidence:      0,
	}

	if parsed.Title == "" {
		result.SuggestedAction = "skip"
		return result
	}

	// Try matching
	match, err := p.matcher.Match(ctx, filepath.Base(filePath))
	if err != nil {
		p.logger.Warn("scan match error", "file", filePath, "error", err)
		result.SuggestedAction = "manual_review"
		return result
	}

	if match.Matched {
		result.MatchedMedia = match.Title
		result.MatchedMediaID = match.MediaID
		result.MediaType = string(match.MediaType)

		// Calculate confidence based on title similarity
		score := titleSimilarity(parsed.Title, match.Title)
		if parsed.Year > 0 && match.Year == parsed.Year {
			score += 20
			if score > 100 {
				score = 100
			}
		}
		result.Confidence = float64(score) / 100.0

		// Determine suggested action
		destFile := filepath.Join(match.DestPath, filepath.Base(filePath))
		if _, err := os.Stat(destFile); err == nil {
			// Destination exists
			result.SuggestedAction = "reimport"
		} else {
			result.SuggestedAction = "import"
		}
	} else {
		result.SuggestedAction = "no_match"
		result.Confidence = 0
	}

	return result
}
