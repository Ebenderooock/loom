package organizer

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
)

// NamingConfig controls how movie files and folders are named.
type NamingConfig struct {
	ID                string `json:"id"`
	MovieFolderFormat string `json:"movie_folder_format"`
	MovieFileFormat   string `json:"movie_file_format"`
	ColonReplacement  string `json:"colon_replacement"`
	RenameMovies      bool   `json:"rename_movies"`
}

// DefaultNamingConfig returns sensible defaults matching Radarr conventions.
func DefaultNamingConfig() *NamingConfig {
	return &NamingConfig{
		ID:                "default",
		MovieFolderFormat: "{Movie Title} ({Release Year})",
		MovieFileFormat:   "{Movie Title} ({Release Year}) [{Quality Full}]",
		ColonReplacement:  " -",
		RenameMovies:      true,
	}
}

// RenamePreview holds the before/after paths for a single file.
type RenamePreview struct {
	FileID      string `json:"file_id"`
	MovieID     string `json:"movie_id"`
	MovieTitle  string `json:"movie_title"`
	CurrentPath string `json:"current_path"`
	NewPath     string `json:"new_path"`
	Changed     bool   `json:"changed"`
	Collision   bool   `json:"collision,omitempty"`
	Error       string `json:"error,omitempty"`
}

// RenameResult holds the outcome of a single file rename.
type RenameResult struct {
	FileID  string `json:"file_id"`
	MovieID string `json:"movie_id"`
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// FormatFolderName renders the folder name for a movie.
func FormatFolderName(movie *movies.Movie, config *NamingConfig) string {
	result := replaceTokens(config.MovieFolderFormat, movie, nil, nil)
	result = applyColonReplacement(result, config.ColonReplacement)
	result = sanitizePathSegment(result)
	return result
}

// FormatFileName renders the file name for a movie file (without extension).
func FormatFileName(movie *movies.Movie, file *movies.MovieFile, config *NamingConfig) string {
	var rel *parser.Release
	if file != nil {
		rel = parser.Parse(filepath.Base(file.FilePath))
	}
	result := replaceTokens(config.MovieFileFormat, movie, file, rel)
	result = applyColonReplacement(result, config.ColonReplacement)
	result = sanitizePathSegment(result)
	return result
}

// BuildTargetPath computes the full target path for a movie file.
func BuildTargetPath(rootPath string, movie *movies.Movie, file *movies.MovieFile, config *NamingConfig) string {
	folder := FormatFolderName(movie, config)
	ext := filepath.Ext(file.FilePath)
	baseName := FormatFileName(movie, file, config)
	return filepath.Join(rootPath, folder, baseName+ext)
}

// replaceTokens substitutes naming tokens with actual values.
func replaceTokens(format string, movie *movies.Movie, file *movies.MovieFile, rel *parser.Release) string {
	result := format

	// Movie tokens
	result = strings.ReplaceAll(result, "{Movie Title}", movie.Title)
	result = strings.ReplaceAll(result, "{Movie CleanTitle}", cleanTitle(movie.Title))
	result = strings.ReplaceAll(result, "{Movie TitleThe}", titleThe(movie.Title))

	// Year
	if movie.Year > 0 {
		result = strings.ReplaceAll(result, "{Release Year}", fmt.Sprintf("%d", movie.Year))
	} else {
		result = strings.ReplaceAll(result, "{Release Year}", "")
	}

	// IDs
	result = strings.ReplaceAll(result, "{IMDB Id}", derefStr(movie.IMDBID))
	result = strings.ReplaceAll(result, "{TMDB Id}", derefStr(movie.TMDBID))

	// Quality tokens (from file or parser)
	qualFull, qualRes, qualSource, qualCodec := extractQualityTokens(file, rel)
	result = strings.ReplaceAll(result, "{Quality Full}", qualFull)
	result = strings.ReplaceAll(result, "{Quality Title}", qualFull) // alias
	result = strings.ReplaceAll(result, "{Quality Resolution}", qualRes)
	result = strings.ReplaceAll(result, "{Quality Source}", qualSource)

	// MediaInfo tokens
	result = strings.ReplaceAll(result, "{MediaInfo VideoCodec}", qualCodec)
	result = strings.ReplaceAll(result, "{MediaInfo AudioCodec}", mediaInfoVal(file, "audio_codec"))
	result = strings.ReplaceAll(result, "{MediaInfo AudioChannels}", mediaInfoVal(file, "audio_channels"))
	result = strings.ReplaceAll(result, "{MediaInfo VideoDynamicRange}", mediaInfoVal(file, "dynamic_range"))

	// Release group (from parser or filename)
	rg := ""
	if rel != nil && rel.Source != "" {
		// Parser doesn't extract release group yet; future enhancement
	}
	result = strings.ReplaceAll(result, "{Release Group}", rg)
	result = strings.ReplaceAll(result, "{Edition Tags}", "")

	// Clean up empty brackets and excess whitespace
	result = cleanupEmptyTokens(result)
	return result
}

// extractQualityTokens derives quality strings from file metadata and parsed release.
func extractQualityTokens(file *movies.MovieFile, rel *parser.Release) (full, res, source, codec string) {
	if file != nil && file.Quality != "" {
		full = file.Quality
	}
	if rel != nil {
		if rel.Resolution > 0 {
			res = fmt.Sprintf("%dp", rel.Resolution)
		}
		source = rel.Source
		codec = rel.Codec
	}
	// Build full quality string if not already set
	if full == "" {
		parts := []string{}
		if source != "" {
			parts = append(parts, source)
		}
		if res != "" {
			parts = append(parts, res)
		}
		if codec != "" {
			parts = append(parts, codec)
		}
		full = strings.Join(parts, " ")
	}
	return
}

// cleanupEmptyTokens removes empty brackets, double spaces, and trailing separators.
var (
	emptyBracketsRe = regexp.MustCompile(`\[\s*\]|\(\s*\)|\{\s*\}`)
	multiSpaceRe    = regexp.MustCompile(`\s{2,}`)
	trailingSepRe   = regexp.MustCompile(`[\s\-_]+$`)
	leadingSepRe    = regexp.MustCompile(`^[\s\-_]+`)
)

func cleanupEmptyTokens(s string) string {
	s = emptyBracketsRe.ReplaceAllString(s, "")
	s = multiSpaceRe.ReplaceAllString(s, " ")
	s = trailingSepRe.ReplaceAllString(s, "")
	s = leadingSepRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// sanitizePathSegment removes characters that are illegal in filenames.
func sanitizePathSegment(s string) string {
	// Remove path separators and illegal filename characters
	illegal := []string{"/", "\\", "<", ">", "|", "\"", "?", "*"}
	for _, ch := range illegal {
		s = strings.ReplaceAll(s, ch, "")
	}
	// Remove control characters
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	// Prevent dots-only names
	if strings.Trim(s, ".") == "" {
		s = "_"
	}
	return strings.TrimSpace(s)
}

// applyColonReplacement replaces colons in names.
func applyColonReplacement(s, replacement string) string {
	return strings.ReplaceAll(s, ":", replacement)
}

// cleanTitle removes articles and special characters for a "clean" version.
func cleanTitle(title string) string {
	t := strings.ToLower(title)
	// Remove leading articles
	for _, article := range []string{"the ", "a ", "an "} {
		t = strings.TrimPrefix(t, article)
	}
	// Remove non-alphanumeric characters except spaces
	t = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			return r
		}
		return -1
	}, t)
	return strings.TrimSpace(t)
}

// titleThe moves "The" to the end, e.g. "The Matrix" -> "Matrix, The"
func titleThe(title string) string {
	lower := strings.ToLower(title)
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lower, article) {
			art := title[:len(article)-1] // preserve original case
			rest := title[len(article):]
			return rest + ", " + art
		}
	}
	return title
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mediaInfoVal(file *movies.MovieFile, key string) string {
	if file == nil || file.MediaInfo == nil {
		return ""
	}
	v, ok := file.MediaInfo[key]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
