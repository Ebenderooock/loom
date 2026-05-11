package imports

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

// Matcher resolves downloaded filenames to library items.
type Matcher struct {
	moviesSvc  movies.Service
	seriesSvc  series.Service
	libStore   *libraries.Store
}

// NewMatcher creates a Matcher backed by the movies and series services.
func NewMatcher(moviesSvc movies.Service, seriesSvc series.Service, libStore *libraries.Store) *Matcher {
	return &Matcher{moviesSvc: moviesSvc, seriesSvc: seriesSvc, libStore: libStore}
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
	// Year in title: (2023) or .2023. or _2023_ or -2023-
	reYear = regexp.MustCompile(`[\.\s\(_\-]?((?:19|20)\d{2})[\.\s\)_\-]?`)
	// Release group suffix: -GROUP at end of name. Must be 4+ alpha chars
	// (optionally followed by digits) to avoid stripping season markers like -S01.
	reGroup = regexp.MustCompile(`(?i)\s*-\s*[a-z]{2,}[a-z0-9]*$`)
)

// parseReleaseName extracts title, year, season, and episode from a release name.
func parseReleaseName(name string) parsedRelease {
	// Remove file extension
	name = strings.TrimSuffix(name, filepath.Ext(name))

	// Strip bracket-enclosed tags early: [1080p], [BluRay], etc.
	name = regexp.MustCompile(`\[([^\]]*)\]`).ReplaceAllStringFunc(name, func(m string) string {
		inner := m[1 : len(m)-1]
		// Keep bracketed content that looks like a year
		if regexp.MustCompile(`^(19|20)\d{2}$`).MatchString(inner) {
			return inner // remove brackets, keep year
		}
		return "" // strip tag
	})

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

	// Extract year — match "(2016)" or ".2016." or " 2016 " etc.
	if m := reYear.FindStringSubmatch(name); len(m) > 1 {
		yearStr := m[1]
		p.Year, _ = strconv.Atoi(yearStr)

		// Remove year from title, including surrounding parens if present.
		// Handle "(2016)", "( 2016 )", and bare "2016".
		yearWithParens := regexp.MustCompile(`\(\s*` + yearStr + `\s*\)`)
		if yearWithParens.MatchString(p.Title) {
			p.Title = yearWithParens.ReplaceAllString(p.Title, "")
		} else {
			// Only strip bare year if it's NOT the first word (avoids eating "2001" from "2001 A Space Odyssey")
			trimmed := strings.TrimSpace(p.Title)
			if !strings.HasPrefix(trimmed, yearStr) {
				p.Title = strings.Replace(p.Title, yearStr, "", 1)
			}
		}
		p.Title = strings.TrimSpace(p.Title)
		p.Title = strings.TrimRight(p.Title, " -.")
	}

	return p
}

// cleanTitle normalises a release title: replaces dots/underscores with
// spaces, collapses runs of whitespace, and strips common codec/quality
// tags from the tail so that "The.Matrix.1999.1080p.BluRay" becomes
// "The Matrix".
func cleanTitle(raw string) string {
	// Strip release group suffix (e.g., "-GROUP") before separator replacement
	s := reGroup.ReplaceAllString(raw, "")

	// Replace dots, underscores, and hyphens with spaces
	s = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(s)

	// Remove common quality/codec tags
	tags := regexp.MustCompile(`(?i)\b(1080p|720p|2160p|4k|bluray|blu ray|bdrip|brrip|webrip|web-dl|webdl|hdtv|dvdrip|x264|x265|h264|h265|hevc|aac|dts|remux|proper|repack|extended|unrated|directors\s*cut|10bit|hdr|uhd|amzn|nf|dsnp|hmax|atmos|multi|dual|complete|season|internal)\b`)
	s = tags.ReplaceAllString(s, "")

	// Remove empty parentheses/brackets left after stripping
	s = regexp.MustCompile(`[\(\[\{]\s*[\)\]\}]`).ReplaceAllString(s, "")

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

// MatchPath tries to match a file using both the filename and parent
// directory name, picking the best result. This handles unorganized
// libraries where the folder name (e.g., "Deadpool (2016)") carries
// more information than the filename (e.g., "movie.mkv").
func (m *Matcher) MatchPath(ctx context.Context, filePath string) (*MatchResult, error) {
	base := filepath.Base(filePath)
	dir := filepath.Base(filepath.Dir(filePath))

	// 1. Try the filename first
	result, err := m.Match(ctx, base)
	if err != nil {
		return nil, err
	}
	if result.Matched {
		return result, nil
	}

	// 2. Try the parent directory name (common for organised/semi-organised libraries)
	if dir != "." && dir != "/" && dir != "" {
		dirResult, err := m.Match(ctx, dir)
		if err != nil {
			return nil, err
		}
		if dirResult.Matched {
			return dirResult, nil
		}
	}

	// 3. Try combining directory + filename for more context
	// e.g., dir="Deadpool" file="2016.mkv" → combined "Deadpool 2016"
	if dir != "." && dir != "/" && dir != "" {
		combined := dir + " " + strings.TrimSuffix(base, filepath.Ext(base))
		combinedResult, err := m.Match(ctx, combined)
		if err != nil {
			return nil, err
		}
		if combinedResult.Matched {
			return combinedResult, nil
		}
	}

	return &MatchResult{Matched: false}, nil
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

	lib, err := m.libStore.Get(ctx, best.LibraryID)
	if err != nil {
		return nil, fmt.Errorf("get library: %w", err)
	}

	destDir := filepath.Join(lib.Path, sanitizeDirName(fmt.Sprintf("%s (%d)", best.Title, best.Year)))
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
	candidates, err := m.seriesSvc.SearchSeries(ctx, p.Title)
	if err != nil {
		return nil, fmt.Errorf("search series: %w", err)
	}

	best := fuzzyMatchSeries(candidates, p)
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

	// Look up the library to get the root path (same pattern as matchMovie)
	lib, err := m.libStore.Get(ctx, best.LibraryID)
	if err != nil {
		return nil, fmt.Errorf("get library: %w", err)
	}

	destDir := filepath.Join(
		lib.Path,
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

// MatchExact resolves a known media item (by type and ID) to a destination
// path without any fuzzy matching. Used for manual matching where the user
// explicitly links a file to a media item.
func (m *Matcher) MatchExact(ctx context.Context, mediaType MediaType, mediaID string) (*MatchResult, error) {
	switch mediaType {
	case MediaTypeMovie:
		movie, err := m.moviesSvc.GetMovie(ctx, mediaID)
		if err != nil {
			return nil, fmt.Errorf("get movie: %w", err)
		}
		lib, err := m.libStore.Get(ctx, movie.LibraryID)
		if err != nil {
			return nil, fmt.Errorf("get library: %w", err)
		}
		destDir := filepath.Join(lib.Path, sanitizeDirName(fmt.Sprintf("%s (%d)", movie.Title, movie.Year)))
		return &MatchResult{
			Matched:   true,
			MediaType: MediaTypeMovie,
			MediaID:   movie.ID,
			Title:     movie.Title,
			Year:      movie.Year,
			DestPath:  destDir,
		}, nil

	case MediaTypeEpisode:
		ep, err := m.seriesSvc.GetEpisode(ctx, mediaID)
		if err != nil {
			return nil, fmt.Errorf("get episode: %w", err)
		}
		show, err := m.seriesSvc.GetSeries(ctx, ep.SeriesID)
		if err != nil {
			return nil, fmt.Errorf("get series: %w", err)
		}
		lib, err := m.libStore.Get(ctx, show.LibraryID)
		if err != nil {
			return nil, fmt.Errorf("get library: %w", err)
		}
		// Find season number from the populated series
		seasonNum := 1
		for _, s := range show.Seasons {
			if s.ID == ep.SeasonID {
				seasonNum = s.SeasonNumber
				break
			}
		}
		destDir := filepath.Join(
			lib.Path,
			sanitizeDirName(show.Title),
			fmt.Sprintf("Season %02d", seasonNum),
		)
		return &MatchResult{
			Matched:   true,
			MediaType: MediaTypeEpisode,
			MediaID:   ep.ID,
			Title:     show.Title,
			Season:    seasonNum,
			Episode:   ep.EpisodeNumber,
			DestPath:  destDir,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported media type: %s", mediaType)
	}
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
