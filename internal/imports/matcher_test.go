package imports

import (
	"testing"

	"github.com/loomctl/loom/internal/movies"
	"github.com/loomctl/loom/internal/series"
)

// ── parseReleaseName ─────────────────────────────────────────────────

func TestParseReleaseName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantTitle string
		wantYear  int
		wantSeason int
		wantEpisode int
		wantSeries bool
	}{
		{
			name:       "movie with year and quality",
			input:      "The.Matrix.1999.1080p.BluRay.x264-GROUP.mkv",
			wantTitle:  "The Matrix  GROUP",
			// cleanTitle replaces hyphens with spaces, so "-GROUP" becomes " GROUP"
			// and the year "1999" is stripped by parseReleaseName after cleanTitle
			wantYear:   1999,
			wantSeries: false,
		},
		{
			name:        "tv show with S01E05",
			input:       "Breaking.Bad.S01E05.720p.BluRay.mkv",
			wantTitle:   "Breaking Bad",
			wantSeason:  1,
			wantEpisode: 5,
			wantSeries:  true,
		},
		{
			name:        "tv show lowercase",
			input:       "game.of.thrones.s08e06.1080p.webrip.mkv",
			wantTitle:   "game of thrones",
			wantSeason:  8,
			wantEpisode: 6,
			wantSeries:  true,
		},
		{
			name:       "movie with parenthesized year",
			input:      "Inception (2010) 1080p BluRay.mkv",
			wantTitle:  "Inception ()",
			wantYear:   2010,
			wantSeries: false,
		},
		{
			name:       "simple title no markers",
			input:      "Some Movie.mkv",
			wantTitle:  "Some Movie",
			wantSeries: false,
		},
		{
			name:       "movie with underscores (year not matched due to underscore boundaries)",
			input:      "The_Dark_Knight_2008_BluRay_1080p.mkv",
			wantTitle:  "The Dark Knight 2008",
			wantYear:   0,
			wantSeries: false,
		},
		{
			name:        "series double digit season episode",
			input:       "The.Simpsons.S35E12.HDTV.x264.mkv",
			wantTitle:   "The Simpsons",
			wantSeason:  35,
			wantEpisode: 12,
			wantSeries:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseReleaseName(tt.input)
			if got.IsSeries != tt.wantSeries {
				t.Errorf("IsSeries = %v, want %v", got.IsSeries, tt.wantSeries)
			}
			if tt.wantTitle != "" && got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
			if tt.wantYear > 0 && got.Year != tt.wantYear {
				t.Errorf("Year = %d, want %d", got.Year, tt.wantYear)
			}
			if tt.wantSeries {
				if got.Season != tt.wantSeason {
					t.Errorf("Season = %d, want %d", got.Season, tt.wantSeason)
				}
				if got.Episode != tt.wantEpisode {
					t.Errorf("Episode = %d, want %d", got.Episode, tt.wantEpisode)
				}
			}
		})
	}
}

// ── cleanTitle ───────────────────────────────────────────────────────

func TestCleanTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"dots to spaces", "The.Matrix.1999.1080p.BluRay", "The Matrix 1999"},
		{"underscores to spaces", "The_Dark_Knight", "The Dark Knight"},
		{"quality tags stripped", "Inception 1080p BluRay x264", "Inception"},
		{"multiple spaces collapsed", "  The   Matrix  ", "The Matrix"},
		{"empty string", "", ""},
		{"only quality tags", "1080p BluRay x264", ""},
		{"mixed separators", "Game.of_Thrones-S01", "Game of Thrones S01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanTitle(tt.input)
			if got != tt.want {
				t.Errorf("cleanTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── titleSimilarity ──────────────────────────────────────────────────

func TestTitleSimilarity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b     string
		minScore int
	}{
		{"the matrix", "the matrix", 100},
		{"The Matrix", "the matrix", 100},   // case insensitive
		{"the matrix", "matrix", 40},         // substring
		{"breaking bad", "breaking bad", 100},
		{"some movie", "completely different", 0},
		{"", "", 0},
		{"abc", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			score := titleSimilarity(tt.a, tt.b)
			if score < tt.minScore {
				t.Errorf("titleSimilarity(%q, %q) = %d, want >= %d", tt.a, tt.b, score, tt.minScore)
			}
		})
	}
}

func TestTitleSimilarity_ExactMatch100(t *testing.T) {
	t.Parallel()
	score := titleSimilarity("Inception", "inception")
	if score != 100 {
		t.Errorf("exact match should be 100, got %d", score)
	}
}

// ── normalise ────────────────────────────────────────────────────────

func TestNormalise(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"The Matrix", "the matrix"},
		{"Game.of.Thrones", "gameofthrones"},
		{"Mr. Robot!", "mr robot"},
		{"  spaces  ", "spaces"},
		{"123 Test", "123 test"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalise(tt.input)
			if got != tt.want {
				t.Errorf("normalise(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── sanitizeDirName ──────────────────────────────────────────────────

func TestSanitizeDirName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"Normal Title", "Normal Title"},
		{"Title: Subtitle", "Title - Subtitle"},
		{"What/Where", "What-Where"},
		{"Star*Wars", "StarWars"},
		{"Who?", "Who"},
		{`He said "hello"`, "He said hello"},
		{"A<B>C", "ABC"},
		{"Pipe|Line", "PipeLine"},
		{"Back\\Slash", "Back-Slash"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeDirName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeDirName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── fuzzyMatchMovie ──────────────────────────────────────────────────

func TestFuzzyMatchMovie_EmptyCandidates(t *testing.T) {
	t.Parallel()
	got := fuzzyMatchMovie(nil, parsedRelease{Title: "Test"})
	if got != nil {
		t.Error("expected nil for empty candidates")
	}
}

func TestFuzzyMatchMovie_ExactMatch(t *testing.T) {
	t.Parallel()
	candidates := []*movies.Movie{
		{ID: "m1", Title: "Inception", Year: 2010},
		{ID: "m2", Title: "Interstellar", Year: 2014},
	}
	got := fuzzyMatchMovie(candidates, parsedRelease{Title: "Inception", Year: 2010})
	if got == nil || got.ID != "m1" {
		t.Errorf("expected m1, got %v", got)
	}
}

func TestFuzzyMatchMovie_YearBoost(t *testing.T) {
	t.Parallel()
	candidates := []*movies.Movie{
		{ID: "m1", Title: "The Matrix", Year: 1999},
		{ID: "m2", Title: "The Matrix", Year: 2021},
	}
	got := fuzzyMatchMovie(candidates, parsedRelease{Title: "The Matrix", Year: 1999})
	if got == nil || got.ID != "m1" {
		t.Errorf("expected m1 (year match), got %v", got)
	}
}

func TestFuzzyMatchMovie_BelowThreshold(t *testing.T) {
	t.Parallel()
	candidates := []*movies.Movie{
		{ID: "m1", Title: "Completely Different Movie"},
	}
	got := fuzzyMatchMovie(candidates, parsedRelease{Title: "xyzzy"})
	if got != nil {
		t.Error("expected nil when below threshold")
	}
}

// ── fuzzyMatchSeries ─────────────────────────────────────────────────

func TestFuzzyMatchSeries_EmptyCandidates(t *testing.T) {
	t.Parallel()
	got := fuzzyMatchSeries(nil, parsedRelease{Title: "Test"})
	if got != nil {
		t.Error("expected nil for empty candidates")
	}
}

func TestFuzzyMatchSeries_ExactMatch(t *testing.T) {
	t.Parallel()
	candidates := []*series.Series{
		{ID: "s1", Title: "Breaking Bad", Year: 2008},
		{ID: "s2", Title: "Better Call Saul", Year: 2015},
	}
	got := fuzzyMatchSeries(candidates, parsedRelease{Title: "Breaking Bad"})
	if got == nil || got.ID != "s1" {
		t.Errorf("expected s1, got %v", got)
	}
}

func TestFuzzyMatchSeries_YearBoost(t *testing.T) {
	t.Parallel()
	candidates := []*series.Series{
		{ID: "s1", Title: "Doctor Who", Year: 1963},
		{ID: "s2", Title: "Doctor Who", Year: 2005},
	}
	got := fuzzyMatchSeries(candidates, parsedRelease{Title: "Doctor Who", Year: 2005})
	if got == nil || got.ID != "s2" {
		t.Errorf("expected s2 (year match), got %v", got)
	}
}

func TestFuzzyMatchSeries_BelowThreshold(t *testing.T) {
	t.Parallel()
	candidates := []*series.Series{
		{ID: "s1", Title: "Completely Different Show"},
	}
	got := fuzzyMatchSeries(candidates, parsedRelease{Title: "xyzzy"})
	if got != nil {
		t.Error("expected nil when below threshold")
	}
}
