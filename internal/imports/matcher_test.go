package imports

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

type contextualSeriesService struct {
	*packSeriesService
	shows     []*series.Series
	seasons   []*series.Season
	episodes  []*series.Episode
	searchErr error
}

func (s *contextualSeriesService) SearchSeries(context.Context, string) ([]*series.Series, error) {
	return s.shows, s.searchErr
}

func (s *contextualSeriesService) GetSeries(_ context.Context, id string) (*series.Series, error) {
	for _, show := range s.shows {
		if show.ID == id {
			return show, nil
		}
	}
	return nil, errors.New("series not found")
}

func (s *contextualSeriesService) GetEpisode(_ context.Context, id string) (*series.Episode, error) {
	for _, ep := range s.episodes {
		if ep.ID == id {
			return ep, nil
		}
	}
	return nil, errors.New("episode not found")
}

func (s *contextualSeriesService) ListSeasons(_ context.Context, id string) ([]*series.Season, error) {
	var out []*series.Season
	for _, season := range s.seasons {
		if season.SeriesID == id {
			out = append(out, season)
		}
	}
	return out, nil
}

func (s *contextualSeriesService) ListEpisodes(_ context.Context, id string, number *int) ([]*series.Episode, error) {
	var out []*series.Episode
	for _, ep := range s.episodes {
		if ep.SeriesID != id {
			continue
		}
		for _, season := range s.seasons {
			if ep.SeasonID == season.ID && (number == nil || season.SeasonNumber == *number) {
				out = append(out, ep)
			}
		}
	}
	return out, nil
}

func dragonBallCatalog(f *packFixture) *contextualSeriesService {
	s := &contextualSeriesService{
		packSeriesService: f.svc,
		shows: []*series.Series{
			{ID: "db", Title: "Dragon Ball", Year: 1986, LibraryID: "tv"},
			{ID: "dbz", Title: "Dragon Ball Z", Year: 1989, LibraryID: "tv"},
		},
		seasons: []*series.Season{
			{ID: "db-specials", SeriesID: "db", SeasonNumber: 0, EpisodeCount: 1},
			{ID: "db-s1", SeriesID: "db", SeasonNumber: 1, EpisodeCount: 13},
			{ID: "db-s2", SeriesID: "db", SeasonNumber: 2, EpisodeCount: 140},
			{ID: "dbz-s1", SeriesID: "dbz", SeasonNumber: 1, EpisodeCount: 1},
		},
	}
	s.episodes = append(s.episodes, &series.Episode{
		ID: "special", SeriesID: "db", SeasonID: "db-specials", EpisodeNumber: 1, Title: "Bonus",
	}, &series.Episode{
		ID: "z1", SeriesID: "dbz", SeasonID: "dbz-s1", EpisodeNumber: 1, Title: "The New Threat",
	})
	for i := 1; i <= 153; i++ {
		season, ep := "db-s1", i
		if i > 13 {
			season, ep = "db-s2", i-13
		}
		title := fmt.Sprintf("Adventure %d", i)
		if i == 1 {
			title = "The Secret of the Dragon Balls"
		}
		s.episodes = append(s.episodes, &series.Episode{
			ID: fmt.Sprintf("db%d", i), SeriesID: "db", SeasonID: season, EpisodeNumber: ep, Title: title,
		})
	}
	f.p.matcher.seriesSvc = s
	return s
}

func TestMatchPathBareEpisodeContext(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	s := dragonBallCatalog(f)
	for _, tc := range []struct {
		path            string
		id              string
		season, episode int
	}{
		{"/downloads/Dragon Ball/Episode 1 The Secret of the Dragon Balls.mkv", "db1", 1, 1},
		{"/downloads/Dragon Ball/Season 02/Episode 1 Adventure 14.mkv", "db14", 2, 1},
		{"/downloads/Dragon Ball The Complete Series (1995\u20132003)/Episode 1 The Secret of the Dragon Balls.mkv", "db1", 1, 1},
		{"/downloads/Dragon Ball/Extras/Dragon Ball Z/Episode 1 The New Threat.mkv", "z1", 1, 1},
		{"/downloads/Dragon Ball and Dragon Ball Z/Dragon Ball/Episode 14 Adventure 14.mkv", "db14", 2, 1},
		{"/downloads/Dragon Ball and Dragon Ball Z/Episode 1 The Secret of the Dragon Balls.mkv", "", 0, 0},
		{"/downloads/Dragon Ball The Complete Series and Dragon Ball Z/Episode 1 The Secret of the Dragon Balls.mkv", "", 0, 0},
		{"/downloads/Dragon Ball/Episode 14 Wrong Title.mkv", "", 0, 0},
		{"/downloads/Dragon Ball/Episode 14.mkv", "", 0, 0},
		{"/downloads/Dragon Ball/Episode 0 Bonus.mkv", "", 0, 0},
		{"/downloads/Dragon Ball/Season 02/Episode 1.mkv", "db14", 2, 1},
		{"/downloads/Unknown/Episode 1 The Secret of the Dragon Balls.mkv", "", 0, 0},
		{"/downloads/Dragon Ball/Dragon.Ball.S02E01.mkv", "db14", 2, 1},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got, err := f.p.matcher.MatchPath(context.Background(), tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if got.Matched != (tc.id != "") || got.MediaID != tc.id {
				t.Fatalf("match = %+v, want %q", got, tc.id)
			}
			if got.Matched && (got.Season != tc.season || got.Episode != tc.episode ||
				filepath.Base(got.DestPath) != fmt.Sprintf("Season %02d", tc.season)) {
				t.Fatalf("wrong destination episode: %+v", got)
			}
		})
	}
	for _, tc := range []struct {
		path, root, title, id string
	}{
		{"/downloads/hash/Episode 14 Adventure 14.mkv", "/downloads/hash", "Dragon Ball", "db14"},
		{"/downloads/Dragon Ball/Dragon Ball Z/Episode 1 The New Threat.mkv", "/downloads/Dragon Ball", "Dragon Ball", "z1"},
		{"/downloads/hash/Unknown/Episode 1 The Secret of the Dragon Balls.mkv", "/downloads/hash", "Dragon Ball", ""},
		{"/downloads/elsewhere/Episode 1 The Secret of the Dragon Balls.mkv", "/downloads/hash", "Dragon Ball", ""},
		{"/downloads/hash/Episode 1 The Secret of the Dragon Balls.mkv", "/downloads/hash", "Dragon Ball and Dragon Ball Z", ""},
		{"/downloads/Dragon Ball and Dragon Ball Z/Episode 1 The Secret of the Dragon Balls.mkv", "/downloads/Dragon Ball and Dragon Ball Z", "Dragon Ball", ""},
	} {
		got, err := f.p.matcher.MatchDownloadPath(context.Background(), tc.path, tc.root, tc.title, "hash")
		if err != nil || got.Matched != (tc.id != "") || got.MediaID != tc.id {
			t.Fatalf("download match %q: %+v, %v; want %q", tc.path, got, err, tc.id)
		}
	}
	s.shows = append(s.shows, &series.Series{ID: "remake", Title: "Dragon Ball", LibraryID: "tv"})
	got, err := f.p.matcher.MatchPath(context.Background(), "/downloads/Dragon Ball/Episode 1 The Secret of the Dragon Balls.mkv")
	if err != nil || got.Matched {
		t.Fatalf("duplicate series title must not choose first: %+v, %v", got, err)
	}
}

func TestContextEpisodeRejectsIncompleteCatalogAndPropagatesErrors(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	s := dragonBallCatalog(f)
	s.episodes = s.episodes[:len(s.episodes)-1]
	got, err := f.p.matcher.MatchPath(context.Background(), "/downloads/Dragon Ball/Episode 14 Adventure 14.mkv")
	if err != nil || got.Matched {
		t.Fatalf("incomplete absolute ordering must not be inferred: %+v, %v", got, err)
	}
	s.searchErr = errors.New("metadata database unavailable")
	_, err = f.p.importDownloadFile(context.Background(), &downloads.DownloadCompletedEvent{
		Title: "Dragon Ball",
	}, "/downloads/Dragon Ball/Episode 14 Adventure 14.mkv", 153)
	if !errors.Is(err, s.searchErr) {
		t.Fatalf("metadata service failure became deterministic no-match: %v", err)
	}
}

// ── parseReleaseName ─────────────────────────────────────────────────

func TestParseReleaseName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		input       string
		wantTitle   string
		wantYear    int
		wantSeason  int
		wantEpisode int
		wantSeries  bool
	}{
		{
			name:       "movie with year and quality",
			input:      "The.Matrix.1999.1080p.BluRay.x264-GROUP.mkv",
			wantTitle:  "The Matrix",
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
			wantTitle:  "Inception",
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
			name:       "movie with underscores and year",
			input:      "The_Dark_Knight_2008_BluRay_1080p.mkv",
			wantTitle:  "The Dark Knight",
			wantYear:   2008,
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
		{
			name:       "year as part of title preserved",
			input:      "2001 A Space Odyssey.mkv",
			wantTitle:  "2001 A Space Odyssey",
			wantYear:   2001,
			wantSeries: false,
		},
		{
			name:       "torrent name with MULTI and DTS",
			input:      "Deadpool.2016.MULTI.1080p.BluRay.x264.DTS-SPARKS.mkv",
			wantTitle:  "Deadpool",
			wantYear:   2016,
			wantSeries: false,
		},
		{
			name:       "bracket tags",
			input:      "Deadpool (2016) [1080p] [BluRay].mkv",
			wantTitle:  "Deadpool",
			wantYear:   2016,
			wantSeries: false,
		},
		{
			name:        "web-dl series with atmos",
			input:       "The.Mandalorian.S02E01.1080p.WEB-DL.DDP5.1.Atmos.x264-GROUP.mkv",
			wantTitle:   "The Mandalorian",
			wantSeason:  2,
			wantEpisode: 1,
			wantSeries:  true,
		},
		{
			name:       "simple name no year",
			input:      "Deadpool.mkv",
			wantTitle:  "Deadpool",
			wantSeries: false,
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
		{"group suffix stripped", "The.Matrix.1999-GROUP", "The Matrix 1999"},
		{"multi tag stripped", "Deadpool.2016.MULTI.1080p", "Deadpool 2016"},
		{"empty parens removed", "Movie () stuff", "Movie stuff"},
		{"empty brackets removed", "Movie [] thing", "Movie thing"},
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
		{"The Matrix", "the matrix", 100}, // case insensitive
		{"the matrix", "matrix", 40},      // substring
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

func TestFuzzyMatchMovie_ConflictingYear(t *testing.T) {
	candidates := make([]*movies.Movie, 0, 2)
	candidates = append(candidates, &movies.Movie{ID: "remake", Title: "Street Fighter", Year: 2026})
	for _, tc := range []struct {
		year int
		want bool
	}{
		{1994, false}, {2024, false}, {2025, true}, {2026, true}, {2027, true}, {2028, false}, {0, true},
	} {
		got := fuzzyMatchMovie(candidates, parsedRelease{Title: "Street Fighter", Year: tc.year})
		if (got != nil) != tc.want {
			t.Fatalf("year %d: want match %v, got %+v", tc.year, tc.want, got)
		}
	}
	candidates = append(candidates, &movies.Movie{ID: "original", Title: "Street Fighter", Year: 1994})
	if got := fuzzyMatchMovie(candidates, parsedRelease{Title: "Street Fighter", Year: 1994}); got == nil || got.ID != "original" {
		t.Fatalf("should match original rather than remake: %+v", got)
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
