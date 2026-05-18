package imports

import (
	"context"
	"fmt"

	"github.com/ebenderooock/loom/internal/alttitles"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

// AltTitleMatcher uses alternate titles from the database to resolve
// release names that don't match the primary title.
type AltTitleMatcher struct {
	altStore  *alttitles.Store
	moviesSvc movies.Service
	seriesSvc series.Service
}

// NewAltTitleMatcher creates an AltTitleMatcher.
func NewAltTitleMatcher(altStore *alttitles.Store, moviesSvc movies.Service, seriesSvc series.Service) *AltTitleMatcher {
	if altStore == nil {
		return nil
	}
	return &AltTitleMatcher{
		altStore:  altStore,
		moviesSvc: moviesSvc,
		seriesSvc: seriesSvc,
	}
}

// MatchMovieByAltTitle tries to match a parsed title against movie
// alternative titles, returning the first movie whose alt title matches.
func (m *AltTitleMatcher) MatchMovieByAltTitle(ctx context.Context, title string, year int) (*movies.Movie, error) {
	if m == nil || m.altStore == nil {
		return nil, nil
	}

	alts, err := m.altStore.SearchByTitle(ctx, title, "movie")
	if err != nil {
		return nil, fmt.Errorf("search alt titles: %w", err)
	}

	var best *movies.Movie
	bestScore := -1

	for _, alt := range alts {
		movie, err := m.moviesSvc.GetMovie(ctx, alt.MediaID)
		if err != nil {
			continue
		}

		score := 50 // base score for alt-title match
		if year > 0 && movie.Year == year {
			score += 50
		}
		if score > bestScore {
			bestScore = score
			best = movie
		}
	}

	return best, nil
}

// MatchSeriesByAltTitle tries to match a parsed title against series
// alternative titles, returning the first series whose alt title matches.
func (m *AltTitleMatcher) MatchSeriesByAltTitle(ctx context.Context, title string, year int) (*series.Series, error) {
	if m == nil || m.altStore == nil {
		return nil, nil
	}

	alts, err := m.altStore.SearchByTitle(ctx, title, "series")
	if err != nil {
		return nil, fmt.Errorf("search alt titles: %w", err)
	}

	var best *series.Series
	bestScore := -1

	for _, alt := range alts {
		show, err := m.seriesSvc.GetSeries(ctx, alt.MediaID)
		if err != nil {
			continue
		}

		score := 50
		if year > 0 && show.Year == year {
			score += 50
		}
		if score > bestScore {
			bestScore = score
			best = show
		}
	}

	return best, nil
}
