package tmdb

import (
	"github.com/ebenderooock/loom/internal/metadata"
	"strconv"
)

const (
	posterCDNURL    = "https://image.tmdb.org/t/p/w342"
	overviewMaxLen  = 1000
)

// mapMovieResponse converts a TMDb MovieResponse to our MovieMetadata.
// Note: TheatricalDate and DigitalDate are populated separately via
// Client.GetReleaseDates — they are not in the basic /movie/{id} response.
func mapMovieResponse(resp *MovieResponse) *metadata.MovieMetadata {
	m := &metadata.MovieMetadata{
		Title:       resp.Title,
		Overview:    cropOverview(resp.Overview),
		PosterPath:  buildPosterURL(resp.PosterPath),
		ReleaseDate: resp.ReleaseDate,
		Runtime:     resp.Runtime,
		Rating:      resp.VoteAverage,
	}

	// Parse year from release date
	if resp.ReleaseDate != "" && len(resp.ReleaseDate) >= 4 {
		if year, err := strconv.Atoi(resp.ReleaseDate[:4]); err == nil {
			m.Year = year
		}
	}

	// Set TMDB ID
	tmdbIDStr := strconv.Itoa(resp.ID)
	m.TMDBID = &tmdbIDStr

	// Set IMDB ID if available
	if resp.IMDBID != "" {
		m.IMDBID = &resp.IMDBID
	}

	// Extract TVDB ID from external_ids (requires append_to_response=external_ids)
	if resp.ExternalIDs != nil && resp.ExternalIDs.TVDBID > 0 {
		tvdbStr := strconv.Itoa(resp.ExternalIDs.TVDBID)
		m.TVDBID = &tvdbStr
	}

	// Extract genres
	for _, g := range resp.Genres {
		m.Genres = append(m.Genres, g.Name)
	}

	return m
}

// mapTVResponse converts a TMDb TVResponse to our SeriesMetadata.
func mapTVResponse(resp *TVResponse) *metadata.SeriesMetadata {
	s := &metadata.SeriesMetadata{
		Title:        resp.Name,
		Overview:     cropOverview(resp.Overview),
		PosterPath:   buildPosterURL(resp.PosterPath),
		FirstAirDate: resp.FirstAirDate,
		Rating:       resp.VoteAverage,
		Seasons:      resp.NumberOfSeasons,
	}

	// Set TMDB ID
	tmdbIDStr := strconv.Itoa(resp.ID)
	s.TMDBID = &tmdbIDStr

	// Set IMDB ID if available (direct field or from external_ids)
	if resp.IMDBID != "" {
		s.IMDBID = &resp.IMDBID
	} else if resp.ExternalIDs != nil && resp.ExternalIDs.IMDBID != "" {
		s.IMDBID = &resp.ExternalIDs.IMDBID
	}

	// Extract TVDB ID from external_ids (requires append_to_response=external_ids)
	if resp.ExternalIDs != nil && resp.ExternalIDs.TVDBID > 0 {
		tvdbStr := strconv.Itoa(resp.ExternalIDs.TVDBID)
		s.TVDBID = &tvdbStr
	}

	// Extract genres
	for _, g := range resp.Genres {
		s.Genres = append(s.Genres, g.Name)
	}

	return s
}

// mapEpisodeResponse converts a TMDb EpisodeResponse to our EpisodeMetadata.
func mapEpisodeResponse(resp *EpisodeResponse) *metadata.EpisodeMetadata {
	e := &metadata.EpisodeMetadata{
		Season:   resp.SeasonNumber,
		Episode:  resp.EpisodeNumber,
		Title:    resp.Name,
		Overview: cropOverview(resp.Overview),
		AirDate:  resp.AirDate,
		Runtime:  resp.Runtime,
		Rating:   resp.VoteAverage,
	}

	// Set TMDB ID
	tmdbIDStr := strconv.Itoa(resp.ID)
	e.TMDBID = &tmdbIDStr

	return e
}

// cropOverview truncates long overview strings to a reasonable length.
// TMDb overviews can be quite long; we limit to 1000 chars.
func cropOverview(overview string) string {
	if len(overview) <= overviewMaxLen {
		return overview
	}
	return overview[:overviewMaxLen]
}

// buildPosterURL constructs a full poster URL from a poster_path.
// If poster_path is empty or the path is already a full URL, returns as-is.
func buildPosterURL(posterPath string) string {
	if posterPath == "" {
		return ""
	}
	// If already a full URL, return as-is
	if len(posterPath) > 4 && posterPath[:4] == "http" {
		return posterPath
	}
	// Prepend CDN URL to relative path
	return posterCDNURL + posterPath
}

// mapCreditsResponse converts a TMDb CreditsResponse to our Credits model.
func mapCreditsResponse(resp *CreditsResponse) *metadata.Credits {
	credits := &metadata.Credits{
		Cast: make([]metadata.CreditPerson, 0, len(resp.Cast)),
		Crew: make([]metadata.CreditPerson, 0, len(resp.Crew)),
	}
	for _, c := range resp.Cast {
		credits.Cast = append(credits.Cast, metadata.CreditPerson{
			ID:          c.ID,
			Name:        c.Name,
			Role:        c.Character,
			Department:  "Acting",
			ProfilePath: c.ProfilePath,
			Order:       c.Order,
		})
	}
	for _, c := range resp.Crew {
		credits.Crew = append(credits.Crew, metadata.CreditPerson{
			ID:          c.ID,
			Name:        c.Name,
			Role:        c.Job,
			Department:  c.Department,
			ProfilePath: c.ProfilePath,
		})
	}
	return credits
}
