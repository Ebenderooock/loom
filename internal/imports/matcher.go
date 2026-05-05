package imports

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/loomctl/loom/internal/movies"
	"github.com/loomctl/loom/internal/series"
)

// Matcher resolves downloaded filenames to library items.
type Matcher struct {
	moviesSvc  movies.Service
	seriesSvc  series.Service
}

// NewMatcher creates a Matcher backed by the movies and series services.
func NewMatcher(moviesSvc movies.Service, seriesSvc series.Service) *Matcher {
	return &Matcher{moviesSvc: moviesSvc, seriesSvc: seriesSvc}
}

// parsedRelease holds the extracted metadata from a release name.
type parsedRelease struct {
	Title   string
	Year    int
	Season  int
	Episode int
	IsSeries bool
}

// Patterns for matching season/episode from release names.
var (
	// S01E02, s01e02
	reSeasonEpisode = regexp.MustCompile(`(?i)[Ss](\d{1,2})[Ee](\d{1,3})`)
	// Year in title: (2023) or .2023.
	reYear = regexp.MustCompile(`[\.\s\(]?((?:19|20)\d{2})[\.\s\)\-]`)
)

// parseReleaseName extracts title, year, season, and episode from a release name.
func parseReleaseName(name string) parsedRelease {
	// Remove file extension
	name = strings.TrimSuffix(name, filepath.Ext(name))

	var p parsedRelease

	// Check for season/episode pattern
	if m := reSeasonEpisode.FindStringSubmatchIndex(name); m != nil {
		p.IsSeries = true
		p.Season, _ = strconv.Atoi(name[m[2]:m[3]])
		p.Episode, _ = strconv.Atoi(name[m[4]:m[5]])
		// Title is everything before the S01E02 pattern
		titlePart := strings.TrimSpace(name[:m[0]])
		p.Title = cleanTitle(titlePart)
	} else {
		p.Title = cleanTitle(name)
	}

	// Extract year
	if m := reYear.FindStringSubmatch(name); len(m) > 1 {
		p.Year, _ = strconv.Atoi(m[1])
		// Remove year from title if present
		p.Title = strings.TrimSpace(strings.Replace(p.Title, m[1], "", 1))
		p.Title = strings.TrimRight(p.Title, " -.")
	}

	return p
}

// cleanTitle normalises a release title: replaces dots/underscores with
// spaces, collapses runs of whitespace, and strips common codec/quality
// tags from the tail so that "The.Matrix.1999.1080p.BluRay" becomes
// "The Matrix".
func cleanTitle(raw string) string {
	// Replace dots, underscores, and hyphens with spaces
	s := strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(raw)

	// Remove common quality/codec tags
	tags := regexp.MustCompile(`(?i)\b(1080p|720p|2160p|4k|bluray|bdrip|brrip|webrip|web-dl|webdl|hdtv|dvdrip|x264|x265|h264|h265|hevc|aac|dts|remux|proper|repack|extended|unrated|directors\s*cut)\b`)
	s = tags.ReplaceAllString(s, "")

	// Collapse whitespace
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// Match attempts to match a file to a movie or series episode.
func (m *Matcher) Match(ctx context.Context, filename string) (*MatchResult, error) {
	parsed := parseReleaseName(filepath.Base(filename))
	if parsed.Title == "" {
		return &MatchResult{Matched: false}, nil
	}

	if parsed.IsSeries {
		return m.matchSeries(ctx, parsed)
	}
	return m.matchMovie(ctx, parsed)
}

// matchMovie searches for a movie matching the parsed release name.
func (m *Matcher) matchMovie(ctx context.Context, p parsedRelease) (*MatchResult, error) {
	results, err := m.moviesSvc.SearchMovies(ctx, p.Title)
	if err != nil {
		return nil, fmt.Errorf("search movies: %w", err)
	}

	best := fuzzyMatchMovie(results, p)
	if best == nil {
		return &MatchResult{Matched: false}, nil
	}

	rootFolder, err := m.moviesSvc.GetRootFolder(ctx, best.RootFolderID)
	if err != nil {
		return nil, fmt.Errorf("get root folder: %w", err)
	}

	destDir := filepath.Join(rootFolder.Path, sanitizeDirName(fmt.Sprintf("%s (%d)", best.Title, best.Year)))
	return &MatchResult{
		Matched:   true,
		MediaType: MediaTypeMovie,
		MediaID:   best.ID,
		Title:     best.Title,
		Year:      best.Year,
		DestPath:  destDir,
	}, nil
}

// matchSeries searches for a series + episode matching the parsed release.
func (m *Matcher) matchSeries(ctx context.Context, p parsedRelease) (*MatchResult, error) {
	allSeries, err := m.seriesSvc.ListSeries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}

	best := fuzzyMatchSeries(allSeries, p)
	if best == nil {
		return &MatchResult{Matched: false}, nil
	}

	// Find the specific episode
	episodes, err := m.seriesSvc.ListEpisodes(ctx, best.ID, &p.Season)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}

	var matchedEp *series.Episode
	for _, ep := range episodes {
		if ep.EpisodeNumber == p.Episode {
			matchedEp = ep
			break
		}
	}
	if matchedEp == nil {
		return &MatchResult{Matched: false}, nil
	}

	destDir := filepath.Join(
		sanitizeDirName(best.Title),
		fmt.Sprintf("Season %02d", p.Season),
	)
	return &MatchResult{
		Matched:   true,
		MediaType: MediaTypeEpisode,
		MediaID:   matchedEp.ID,
		Title:     best.Title,
		Year:      best.Year,
		Season:    p.Season,
		Episode:   p.Episode,
		DestPath:  destDir,
	}, nil
}

// fuzzyMatchMovie picks the best movie match by title similarity and year.
func fuzzyMatchMovie(candidates []*movies.Movie, p parsedRelease) *movies.Movie {
	if len(candidates) == 0 {
		return nil
	}

	var best *movies.Movie
	bestScore := -1

	for _, m := range candidates {
		score := titleSimilarity(m.Title, p.Title)
		if p.Year > 0 && m.Year == p.Year {
			score += 50
		}
		if score > bestScore {
			bestScore = score
			best = m
		}
	}

	// Require a minimum similarity threshold
	if bestScore < 40 {
		return nil
	}
	return best
}

// fuzzyMatchSeries picks the best series match by title similarity.
func fuzzyMatchSeries(candidates []*series.Series, p parsedRelease) *series.Series {
	if len(candidates) == 0 {
		return nil
	}

	var best *series.Series
	bestScore := -1

	for _, s := range candidates {
		score := titleSimilarity(s.Title, p.Title)
		if p.Year > 0 && s.Year == p.Year {
			score += 50
		}
		if score > bestScore {
			bestScore = score
			best = s
		}
	}

	if bestScore < 40 {
		return nil
	}
	return best
}

// titleSimilarity returns a 0-100 score for how similar two titles are.
func titleSimilarity(a, b string) int {
	a = normalise(a)
	b = normalise(b)
	if a == b {
		return 100
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		shorter := len(a)
		if len(b) < shorter {
			shorter = len(b)
		}
		longer := len(a)
		if len(b) > longer {
			longer = len(b)
		}
		if longer == 0 {
			return 0
		}
		return shorter * 100 / longer
	}

	// Word overlap score
	aWords := strings.Fields(a)
	bWords := strings.Fields(b)
	if len(aWords) == 0 || len(bWords) == 0 {
		return 0
	}
	matches := 0
	for _, aw := range aWords {
		for _, bw := range bWords {
			if aw == bw {
				matches++
				break
			}
		}
	}
	total := len(aWords)
	if len(bWords) > total {
		total = len(bWords)
	}
	return matches * 100 / total
}

// normalise lowercases and strips non-alphanumeric characters (except spaces).
func normalise(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// sanitizeDirName replaces characters that are invalid in directory names.
func sanitizeDirName(name string) string {
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
	)
	return replacer.Replace(name)
}
