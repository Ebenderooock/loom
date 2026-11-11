package imports

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ebenderooock/loom/internal/mediafiles"
)

// ScanResult describes a potential match found during a folder scan.
type ScanResult struct {
	FilePath        string  `json:"file_path"`
	FileSize        int64   `json:"file_size"`
	DetectedTitle   string  `json:"detected_title"`
	DetectedYear    int     `json:"detected_year,omitempty"`
	DetectedSeason  int     `json:"detected_season,omitempty"`
	DetectedEpisode int     `json:"detected_episode,omitempty"`
	DetectedArtist  string  `json:"detected_artist,omitempty"`
	DetectedAlbum   string  `json:"detected_album,omitempty"`
	DetectedTrack   int     `json:"detected_track,omitempty"`
	DetectedDisc    int     `json:"detected_disc,omitempty"`
	MatchedMedia    string  `json:"matched_media,omitempty"`
	MatchedMediaID  string  `json:"matched_media_id,omitempty"`
	MediaType       string  `json:"media_type,omitempty"`
	Confidence      float64 `json:"confidence"`
	SuggestedAction string  `json:"suggested_action"`
	Quality         string  `json:"quality,omitempty"`
	// History reports "already_imported" for same-file identity and
	// "previously_imported" for metadata-only hints, never content proof.
	// Other states reflect the latest queue outcome.
	ImportStatus string `json:"import_status,omitempty"`
	ImportError  string `json:"import_error,omitempty"`
	ImportJobID  string `json:"import_job_id,omitempty"`
}

// ScanFolder scans a directory for media files and returns potential matches
// without actually importing anything. Useful for previewing before import.
func (p *ImportPipeline) ScanFolder(ctx context.Context, path string) ([]ScanResult, error) {
	mediaFiles, err := scanImportFiles(ctx, path)
	if err != nil {
		return nil, err
	}

	var results []ScanResult
	for _, mf := range mediaFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result := p.scanSingleFile(ctx, mf)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

func scanImportFiles(ctx context.Context, path string) ([]string, error) {
	return discoverImportFiles(ctx, path, nil)
}

func discoverImportFiles(ctx context.Context, path string, discovered func(string)) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect import path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing symlink import path: %s", path)
	}
	var files []string
	err = filepath.WalkDir(path, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !mediaExtensions[ext] && !mediafiles.IsAudio(ext) {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if fileInfo.Mode().IsRegular() {
			files = append(files, path)
			if discovered != nil {
				discovered(path)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan import files: %w", err)
	}
	if !info.IsDir() && len(files) == 0 {
		return nil, fmt.Errorf("not a media file: %s", path)
	}
	return files, nil
}

// scanSingleFile analyses a single file and attempts to match it.
func (p *ImportPipeline) scanSingleFile(ctx context.Context, filePath string) ScanResult {
	info, _ := os.Stat(filePath)
	if mediafiles.IsAudio(strings.ToLower(filepath.Ext(filePath))) {
		if info != nil {
			if prior, state := p.scanPriorImport(ctx, filePath, info); prior != nil {
				return ScanResult{
					FilePath: filePath, FileSize: info.Size(), MediaType: string(MediaTypeMusic),
					DetectedTitle:   strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath)),
					MatchedMedia:    filepath.Base(prior.DestPath),
					MatchedMediaID:  prior.MediaID,
					Confidence:      1,
					SuggestedAction: state,
					ImportStatus:    state,
				}
			}
		}
		result := p.scanMusicFile(ctx, filePath)
		result.ImportStatus, result.ImportError, result.ImportJobID = p.fileImportState(ctx, filePath)
		return result
	}
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

	if prior, state := p.scanPriorImport(ctx, filePath, info); prior != nil {
		parsed := parseReleaseName(filepath.Base(filePath))
		return ScanResult{
			FilePath:        filePath,
			FileSize:        fileSize,
			DetectedTitle:   parsed.Title,
			DetectedYear:    parsed.Year,
			DetectedSeason:  parsed.Season,
			DetectedEpisode: parsed.Episode,
			MatchedMedia:    filepath.Base(prior.DestPath),
			MatchedMediaID:  prior.MediaID,
			MediaType:       string(prior.MediaType),
			Confidence:      1,
			SuggestedAction: state,
			ImportStatus:    state,
		}
	}
	importStatus, importErr, jobID := p.fileImportState(ctx, filePath)

	result := ScanResult{
		ImportStatus:    importStatus,
		ImportError:     importErr,
		ImportJobID:     jobID,
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

	// Try matching using full path (includes parent directory)
	match, err := p.matcher.MatchPath(ctx, filePath)
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
