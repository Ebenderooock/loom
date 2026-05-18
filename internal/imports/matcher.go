package imports

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/series"
)

// Matcher resolves downloaded filenames to library items.
type Matcher struct {
	moviesSvc  movies.Service
	seriesSvc  series.Service
	libStore   *libraries.Store
	altMatcher *AltTitleMatcher
}

// NewMatcher creates a Matcher backed by the movies and series services.
func NewMatcher(moviesSvc movies.Service, seriesSvc series.Service, libStore *libraries.Store) *Matcher {
	return &Matcher{moviesSvc: moviesSvc, seriesSvc: seriesSvc, libStore: libStore}
}

// SetAltTitleMatcher installs an alternative-title fallback matcher.
func (m *Matcher) SetAltTitleMatcher(alt *AltTitleMatcher) {
	m.altMatcher = alt
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
	// Release group suffix: -GROUP at end of name. Must be 4+ alpha chars
	// (optionally followed by digits) to avoid stripping season markers like -S01.
	reGroup = regexp.MustCompile(`(?i)\s*-\s*[a-z]{2,}[a-z0-9]*$`)
)

// parseReleaseName extracts title, year, season, and episode from a release name.
// Delegates to the canonical parser package for extraction, then maps to the
// local parsedRelease struct used by the matcher.
func parseReleaseName(name string) parsedRelease {
	r := parser.Parse(name)

	var p parsedRelease
	p.Year = r.Year
	p.Season = r.Season
	p.Episode = r.Episode

	// Determine if this is a series (has season/episode info, season pack, daily date, or absolute episode)
	if r.Season >= 0 || r.Episode >= 0 || r.IsSeasonPack || r.DailyDate != "" || r.AbsoluteEpisode >= 0 {
		p.IsSeries = true
	}

	// Use the parser's cleaned title, but fall back to our own cleanTitle
	// for backward compatibility with the matcher's title-cleaning logic
	// (strip year, quality tags differently for fuzzy matching).
	p.Title = cleanTitleFromParser(name, r)

	return p
}

// cleanTitleFromParser builds a clean title for matching using parser results.
func cleanTitleFromParser(name string, r *parser.Release) string {
	// Remove file extension
	name = strings.TrimSuffix(name, filepath.Ext(name))

	// Strip bracket-enclosed tags early: [1080p], [BluRay], etc.
	name = regexp.MustCompile(`\[([^\]]*)\]`).ReplaceAllStringFunc(name, func(m string) string {
		inner := m[1 : len(m)-1]
		if regexp.MustCompile(`^(19|20)\d{2}$`).MatchString(inner) {
			return inner
		}
		return ""
	})

	var title string

	// If series with S##E## pattern, title is everything before it
	seRe := regexp.MustCompile(`(?i)[Ss](\d{1,2})[Ee](\d{1,3})`)
	if m := seRe.FindStringSubmatchIndex(name); m != nil && r.Season >= 0 && r.Episode >= 0 {
		titlePart := strings.TrimSpace(name[:m[0]])
		title = cleanTitle(titlePart)
	} else {
		title = cleanTitle(name)
	}

	// Extract year string for removal
	yearRe := regexp.MustCompile(`[\.\s\(_\-]?((?:19|20)\d{2})[\.\s\)_\-]?`)
	if m := yearRe.FindStringSubmatch(name); len(m) > 1 && r.Year > 0 {
		yearStr := m[1]
		yearWithParens := regexp.MustCompile(`\(\s*` + yearStr + `\s*\)`)
		if yearWithParens.MatchString(title) {
			title = yearWithParens.ReplaceAllString(title, "")
		} else {
			trimmed := strings.TrimSpace(title)
			if !strings.HasPrefix(trimmed, yearStr) {
				title = strings.Replace(title, yearStr, "", 1)
			}
		}
		title = strings.TrimSpace(title)
		title = strings.TrimRight(title, " -.")
	}

	return title
}

// cleanTitle normalises a release title: replaces dots/underscores with
// spaces, collapses runs of whitespace, and strips common codec/quality
// tags from the tail so that "The.Matrix.1999.1080p.BluRay" becomes
// "The Matrix".
func cleanTitle(raw string) string {
	// Strip release group suffix (e.g., "-GROUP") before separator replacement
	s := reGroup.ReplaceAllString(raw, "")

	// Collapse acronyms (e.g., "M.I.A" → "MIA") before dot-to-space replacement
	s = collapseAcronyms(s)

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
		// Fallback: try alternative titles
		if m.altMatcher != nil {
			altMovie, err := m.altMatcher.MatchMovieByAltTitle(ctx, p.Title, p.Year)
			if err != nil {
				return nil, fmt.Errorf("alt title match: %w", err)
			}
			if altMovie != nil {
				best = altMovie
			}
		}
	}
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
		// Fallback: try alternative titles
		if m.altMatcher != nil {
			altSeries, err := m.altMatcher.MatchSeriesByAltTitle(ctx, p.Title, p.Year)
			if err != nil {
				return nil, fmt.Errorf("alt title match: %w", err)
			}
			if altSeries != nil {
				best = altSeries
			}
		}
	}
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
// Uses a combination of exact match, substring containment, and token
// overlap with stop-word removal. Takes the best score across methods.
func titleSimilarity(a, b string) int {
	na := normalise(a)
	nb := normalise(b)

	// Exact normalised match
	if na == nb {
		return 100
	}

	best := 0

	// Substring containment (character-level)
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		shorter := len(na)
		if len(nb) < shorter {
			shorter = len(nb)
		}
		longer := len(na)
		if len(nb) > longer {
			longer = len(nb)
		}
		if longer > 0 {
			score := shorter * 100 / longer
			if score > best {
				best = score
			}
		}
	}

	// Token-level scoring with stop words removed
	aToks := tokenize(a)
	bToks := tokenize(b)
	if len(aToks) == 0 || len(bToks) == 0 {
		return best
	}

	// Build sets for accurate intersection
	aSet := make(map[string]bool, len(aToks))
	for _, w := range aToks {
		aSet[w] = true
	}
	bSet := make(map[string]bool, len(bToks))
	for _, w := range bToks {
		bSet[w] = true
	}

	intersection := 0
	for w := range aSet {
		if bSet[w] {
			intersection++
		}
	}

	// Containment: fraction of the SMALLER set found in the larger.
	// Handles "Punisher" matching "Marvels Punisher" (1/1 = 100%).
	minSize := len(aSet)
	if len(bSet) < minSize {
		minSize = len(bSet)
	}
	if minSize > 0 {
		containment := intersection * 100 / minSize
		if containment > best {
			best = containment
		}
	}

	// Jaccard: intersection / union. Penalizes noisy titles.
	union := len(aSet)
	for w := range bSet {
		if !aSet[w] {
			union++
		}
	}
	if union > 0 {
		jaccard := intersection * 100 / union
		if jaccard > best {
			best = jaccard
		}
	}

	return best
}

// normalise lowercases, expands common symbols, strips possessives,
// and removes non-alphanumeric characters (except spaces).
func normalise(s string) string {
	s = strings.ToLower(s)
	// Collapse acronyms: "m.i.a" → "mia", "m i a" → "mia"
	s = collapseAcronyms(s)
	// Expand meaningful punctuation before stripping
	s = strings.ReplaceAll(s, "&", " and ")
	// Strip possessives: "marvel's" → "marvels"
	s = strings.ReplaceAll(s, "'s", "s")
	s = strings.ReplaceAll(s, "\u2019s", "s") // curly apostrophe
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	result := strings.TrimSpace(b.String())
	// Collapse whitespace
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	return result
}

// stopWords are common words removed from token-level scoring to avoid
// diluting match quality.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true,
	"of": true, "in": true, "to": true, "for": true, "is": true,
}

// tokenize splits a normalised string into meaningful words, removing stop words.
func tokenize(s string) []string {
	words := strings.Fields(normalise(s))
	out := make([]string, 0, len(words))
	for _, w := range words {
		if !stopWords[w] && len(w) > 0 {
			out = append(out, w)
		}
	}
	return out
}

// collapseAcronyms replaces dot-separated single-letter sequences
// (e.g., "M.I.A" → "MIA", "S.H.I.E.L.D." → "SHIELD") and also
// space-separated single-letter sequences (e.g., "M I A" → "MIA")
// so they match regardless of separator style.
func collapseAcronyms(s string) string {
	// Dot-separated: "M.I.A" or "M.I.A."
	dotRe := regexp.MustCompile(`(?:^|[^A-Za-z])((?:[A-Za-z]\.){2,}[A-Za-z]?)`)
	s = dotRe.ReplaceAllStringFunc(s, func(m string) string {
		prefix := ""
		start := 0
		if len(m) > 0 && !unicode.IsLetter(rune(m[0])) {
			prefix = string(m[0])
			start = 1
		}
		return prefix + strings.ReplaceAll(m[start:], ".", "")
	})

	// Space-separated single letters: "M I A" but not "A Beautiful Mind"
	// Only match if ALL tokens are single letters and there are 2+
	spaceRe := regexp.MustCompile(`(?:^|[^A-Za-z])((?:[A-Za-z] ){2,}[A-Za-z])(?:[^A-Za-z]|$)`)
	s = spaceRe.ReplaceAllStringFunc(s, func(m string) string {
		prefix := ""
		suffix := ""
		start := 0
		end := len(m)
		if len(m) > 0 && !unicode.IsLetter(rune(m[0])) {
			prefix = string(m[0])
			start = 1
		}
		if end > 0 && !unicode.IsLetter(rune(m[end-1])) {
			suffix = string(m[end-1])
			end--
		}
		return prefix + strings.ReplaceAll(m[start:end], " ", "") + suffix
	})

	return s
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
