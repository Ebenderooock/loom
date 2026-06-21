package imports

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ebenderooock/loom/internal/alttitles"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"

	_ "modernc.org/sqlite"
)

// stubMoviesSvc implements the subset of movies.Service needed by AltTitleMatcher.
type stubMoviesSvc struct {
	movies.Service
	moviesByID map[string]*movies.Movie
}

func (s *stubMoviesSvc) GetMovie(_ context.Context, id string) (*movies.Movie, error) {
	if m, ok := s.moviesByID[id]; ok {
		return m, nil
	}
	return nil, sql.ErrNoRows
}

// stubSeriesSvc implements the subset of series.Service needed by AltTitleMatcher.
type stubSeriesSvc struct {
	series.Service
	seriesByID map[string]*series.Series
}

func (s *stubSeriesSvc) GetSeries(_ context.Context, id string) (*series.Series, error) {
	if sv, ok := s.seriesByID[id]; ok {
		return sv, nil
	}
	return nil, sql.ErrNoRows
}

func setupAltTitleDB(t *testing.T) (*sql.DB, *alttitles.Store) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS alternate_titles (
		id TEXT PRIMARY KEY,
		media_id TEXT NOT NULL,
		media_type TEXT NOT NULL,
		title TEXT NOT NULL,
		language TEXT DEFAULT 'en',
		source TEXT DEFAULT 'manual',
		created_at TEXT NOT NULL,
		UNIQUE(media_id, title)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	return db, alttitles.NewStore(db)
}

func TestAltTitleMatcher_MatchMovieByAltTitle(t *testing.T) {
	t.Parallel()
	_, store := setupAltTitleDB(t)
	ctx := context.Background()

	// Seed an alt title
	err := store.Create(ctx, &alttitles.AltTitle{
		MediaID:   "movie-123",
		MediaType: "movie",
		Title:     "Die Hard: With a Vengeance",
		Language:  "en",
		Source:    "tmdb",
	})
	if err != nil {
		t.Fatal(err)
	}

	movieSvc := &stubMoviesSvc{moviesByID: map[string]*movies.Movie{
		"movie-123": {ID: "movie-123", Title: "Die Hard 3", Year: 1995},
	}}
	seriesSvc := &stubSeriesSvc{seriesByID: map[string]*series.Series{}}

	matcher := NewAltTitleMatcher(store, movieSvc, seriesSvc)
	if matcher == nil {
		t.Fatal("expected non-nil matcher")
	}

	got, err := matcher.MatchMovieByAltTitle(ctx, "Die Hard: With a Vengeance", 1995)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected a movie match, got nil")
	}
	if got.ID != "movie-123" {
		t.Fatalf("expected movie-123, got %s", got.ID)
	}
}

func TestAltTitleMatcher_MatchSeriesByAltTitle(t *testing.T) {
	t.Parallel()
	_, store := setupAltTitleDB(t)
	ctx := context.Background()

	err := store.Create(ctx, &alttitles.AltTitle{
		MediaID:   "series-456",
		MediaType: "series",
		Title:     "AoT",
		Language:  "en",
		Source:    "manual",
	})
	if err != nil {
		t.Fatal(err)
	}

	movieSvc := &stubMoviesSvc{moviesByID: map[string]*movies.Movie{}}
	seriesSvc := &stubSeriesSvc{seriesByID: map[string]*series.Series{
		"series-456": {ID: "series-456", Title: "Attack on Titan", Year: 2013},
	}}

	matcher := NewAltTitleMatcher(store, movieSvc, seriesSvc)
	got, err := matcher.MatchSeriesByAltTitle(ctx, "AoT", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected a series match, got nil")
	}
	if got.ID != "series-456" {
		t.Fatalf("expected series-456, got %s", got.ID)
	}
}

func TestAltTitleMatcher_NoMatch(t *testing.T) {
	t.Parallel()
	_, store := setupAltTitleDB(t)
	ctx := context.Background()

	movieSvc := &stubMoviesSvc{moviesByID: map[string]*movies.Movie{}}
	seriesSvc := &stubSeriesSvc{seriesByID: map[string]*series.Series{}}

	matcher := NewAltTitleMatcher(store, movieSvc, seriesSvc)
	got, err := matcher.MatchMovieByAltTitle(ctx, "NonExistentTitle", 2020)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil match for unknown title, got %+v", got)
	}
}
