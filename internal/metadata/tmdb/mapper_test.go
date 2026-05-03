package tmdb

import (
	"strings"
	"testing"
)

func TestMapMovieResponseBasic(t *testing.T) {
	resp := &MovieResponse{
		ID:          550,
		IMDBID:      "tt0137523",
		Title:       "Fight Club",
		ReleaseDate: "1999-10-15",
		Overview:    "An insomniac office worker and a devil-may-care soap maker form an underground fight club.",
		PosterPath:  "/a28my1q3o1MjID6a8ynT2Yemzj.jpg",
		Runtime:     139,
		VoteAverage: 8.8,
		Genres: []GenreResponse{
			{ID: 18, Name: "Drama"},
			{ID: 28, Name: "Action"},
		},
	}

	m := mapMovieResponse(resp)

	if m.Title != "Fight Club" {
		t.Errorf("expected title 'Fight Club', got %q", m.Title)
	}
	if m.Year != 1999 {
		t.Errorf("expected year 1999, got %d", m.Year)
	}
	if m.Runtime != 139 {
		t.Errorf("expected runtime 139, got %d", m.Runtime)
	}
	if m.IMDBID == nil || *m.IMDBID != "tt0137523" {
		t.Errorf("expected IMDB ID 'tt0137523', got %v", m.IMDBID)
	}
	if m.TMDBID == nil || *m.TMDBID != "550" {
		t.Errorf("expected TMDB ID '550', got %v", m.TMDBID)
	}
	if len(m.Genres) != 2 || m.Genres[0] != "Drama" {
		t.Errorf("expected genres [Drama, Action], got %v", m.Genres)
	}
}

func TestMapMovieResponseNoIMDB(t *testing.T) {
	resp := &MovieResponse{
		ID:          550,
		IMDBID:      "",
		Title:       "Fight Club",
		ReleaseDate: "1999-10-15",
	}

	m := mapMovieResponse(resp)

	if m.IMDBID != nil {
		t.Errorf("expected nil IMDB ID, got %v", m.IMDBID)
	}
}

func TestMapMovieResponseBadYear(t *testing.T) {
	resp := &MovieResponse{
		ID:          550,
		Title:       "Test",
		ReleaseDate: "not-a-year-01-01",
	}

	m := mapMovieResponse(resp)

	if m.Year != 0 {
		t.Errorf("expected year 0 for unparseable date, got %d", m.Year)
	}
}

func TestMapMovieResponseEmptyReleaseDate(t *testing.T) {
	resp := &MovieResponse{
		ID:          550,
		Title:       "Test",
		ReleaseDate: "",
	}

	m := mapMovieResponse(resp)

	if m.Year != 0 {
		t.Errorf("expected year 0 for empty date, got %d", m.Year)
	}
}

func TestMapTVResponseBasic(t *testing.T) {
	resp := &TVResponse{
		ID:              1399,
		IMDBID:          "tt0944947",
		Name:            "Game of Thrones",
		FirstAirDate:    "2011-04-18",
		Overview:        "Seven noble families fight for control of the mythical land of Westeros.",
		PosterPath:      "/u3bZgnrm11QwQ5kCD7nau7Sw1qb.jpg",
		VoteAverage:     9.2,
		NumberOfSeasons: 8,
		Genres: []GenreResponse{
			{ID: 18, Name: "Drama"},
			{ID: 10759, Name: "Action & Adventure"},
		},
	}

	s := mapTVResponse(resp)

	if s.Title != "Game of Thrones" {
		t.Errorf("expected title 'Game of Thrones', got %q", s.Title)
	}
	if s.Seasons != 8 {
		t.Errorf("expected 8 seasons, got %d", s.Seasons)
	}
	if s.IMDBID == nil || *s.IMDBID != "tt0944947" {
		t.Errorf("expected IMDB ID 'tt0944947', got %v", s.IMDBID)
	}
	if len(s.Genres) != 2 {
		t.Errorf("expected 2 genres, got %d", len(s.Genres))
	}
}

func TestMapEpisodeResponseBasic(t *testing.T) {
	resp := &EpisodeResponse{
		ID:            349232,
		Name:          "Winter is Coming",
		Overview:      "Bran Stark and his father escort a deserter from the Wall.",
		AirDate:       "2011-04-18",
		EpisodeNumber: 1,
		SeasonNumber:  1,
		Runtime:       56,
		VoteAverage:   7.8,
	}

	ep := mapEpisodeResponse(resp)

	if ep.Title != "Winter is Coming" {
		t.Errorf("expected title 'Winter is Coming', got %q", ep.Title)
	}
	if ep.Season != 1 {
		t.Errorf("expected season 1, got %d", ep.Season)
	}
	if ep.Episode != 1 {
		t.Errorf("expected episode 1, got %d", ep.Episode)
	}
	if ep.Runtime != 56 {
		t.Errorf("expected runtime 56, got %d", ep.Runtime)
	}
	if ep.TMDBID == nil || *ep.TMDBID != "349232" {
		t.Errorf("expected TMDB ID '349232', got %v", ep.TMDBID)
	}
}

func TestCropOverviewShort(t *testing.T) {
	overview := "A short overview"
	cropped := cropOverview(overview)
	if cropped != overview {
		t.Errorf("expected unchanged overview for short text, got %q", cropped)
	}
}

func TestCropOverviewLong(t *testing.T) {
	overview := strings.Repeat("x", 2000)
	cropped := cropOverview(overview)
	if len(cropped) != 1000 {
		t.Errorf("expected cropped length 1000, got %d", len(cropped))
	}
	if cropped != overview[:1000] {
		t.Errorf("expected first 1000 chars, got different content")
	}
}

func TestCropOverviewEmpty(t *testing.T) {
	cropped := cropOverview("")
	if cropped != "" {
		t.Errorf("expected empty string, got %q", cropped)
	}
}

func TestBuildPosterURLRelative(t *testing.T) {
	url := buildPosterURL("/a28my1q3o1MjID6a8ynT2Yemzj.jpg")
	expected := "https://image.tmdb.org/t/p/w342/a28my1q3o1MjID6a8ynT2Yemzj.jpg"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestBuildPosterURLEmpty(t *testing.T) {
	url := buildPosterURL("")
	if url != "" {
		t.Errorf("expected empty string, got %q", url)
	}
}

func TestBuildPosterURLAbsolute(t *testing.T) {
	fullURL := "https://example.com/poster.jpg"
	url := buildPosterURL(fullURL)
	if url != fullURL {
		t.Errorf("expected %q, got %q", fullURL, url)
	}
}

func TestBuildPosterURLHTTP(t *testing.T) {
	fullURL := "http://example.com/poster.jpg"
	url := buildPosterURL(fullURL)
	if url != fullURL {
		t.Errorf("expected %q, got %q", fullURL, url)
	}
}

func TestMapperOverviewCroppingIntegration(t *testing.T) {
	longOverview := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 30)
	resp := &MovieResponse{
		ID:       1,
		Title:    "Long Overview Movie",
		Overview: longOverview,
	}

	m := mapMovieResponse(resp)

	if len(m.Overview) > 1000 {
		t.Errorf("overview exceeds max length 1000: got %d", len(m.Overview))
	}
	if len(m.Overview) != 1000 {
		t.Errorf("expected length 1000, got %d", len(m.Overview))
	}
}

func TestMapMovieResponseNoGenres(t *testing.T) {
	resp := &MovieResponse{
		ID:    1,
		Title: "No Genres Movie",
	}

	m := mapMovieResponse(resp)

	if len(m.Genres) != 0 {
		t.Errorf("expected 0 genres, got %d", len(m.Genres))
	}
}

func TestMapTVResponseNoGenres(t *testing.T) {
	resp := &TVResponse{
		ID:   1,
		Name: "No Genres Series",
	}

	s := mapTVResponse(resp)

	if len(s.Genres) != 0 {
		t.Errorf("expected 0 genres, got %d", len(s.Genres))
	}
}

func TestMapMovieResponseZeroRuntime(t *testing.T) {
	resp := &MovieResponse{
		ID:      1,
		Title:   "Test",
		Runtime: 0,
	}

	m := mapMovieResponse(resp)

	if m.Runtime != 0 {
		t.Errorf("expected runtime 0, got %d", m.Runtime)
	}
}

func TestMapMovieResponseZeroRating(t *testing.T) {
	resp := &MovieResponse{
		ID:          1,
		Title:       "Test",
		VoteAverage: 0,
	}

	m := mapMovieResponse(resp)

	if m.Rating != 0.0 {
		t.Errorf("expected rating 0, got %f", m.Rating)
	}
}

func TestMapMovieResponseHighRating(t *testing.T) {
	resp := &MovieResponse{
		ID:          1,
		Title:       "Test",
		VoteAverage: 10.0,
	}

	m := mapMovieResponse(resp)

	if m.Rating != 10.0 {
		t.Errorf("expected rating 10.0, got %f", m.Rating)
	}
}
