package tvdb

import (
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/metadata"
)

// MapSeriesToMetadata converts TVDB SeriesData to metadata.SeriesMetadata.
func MapSeriesToMetadata(data *SeriesData) *metadata.SeriesMetadata {
	if data == nil {
		return nil
	}

	tvdbIDStr := strconv.Itoa(data.ID)
	m := &metadata.SeriesMetadata{
		TVDBID:       &tvdbIDStr,
		Title:        data.Name,
		Overview:     data.Overview,
		PosterPath:   data.Image,
		FirstAirDate: data.FirstAirDate,
		Genres:       []string{},
	}

	// Extract year from year string (TVDB returns as string like "2008")
	if data.Year != "" {
		if year, err := strconv.Atoi(data.Year); err == nil {
			m.Seasons = year // Using Seasons field for year as fallback
		}
	}

	// Extract IMDB ID if available
	if data.ExternalIDs.IMDB != "" {
		m.IMDBID = &data.ExternalIDs.IMDB
	}

	return m
}

// MapEpisodeToMetadata converts TVDB EpisodeData to metadata.EpisodeMetadata.
func MapEpisodeToMetadata(data *EpisodeData) *metadata.EpisodeMetadata {
	if data == nil {
		return nil
	}

	tvdbIDStr := strconv.Itoa(data.ID)
	m := &metadata.EpisodeMetadata{
		TVDBID:   &tvdbIDStr,
		Season:   data.SeasonNumber,
		Episode:  data.EpisodeNumber,
		Title:    data.Name,
		Overview: data.Overview,
		AirDate:  data.AirDate,
		Runtime:  data.Runtime,
	}

	// Extract IMDB rating if available
	if data.Ratings.IMDB > 0 {
		m.Rating = data.Ratings.IMDB
	}

	return m
}

// MapSearchResultToMetadata converts TVDB SearchResult to metadata.SeriesMetadata.
// Only returns data for series type results.
func MapSearchResultToMetadata(result *SearchResult) *metadata.SeriesMetadata {
	if result == nil || result.Type != "series" {
		return nil
	}

	tvdbIDStr := strconv.Itoa(result.ID)
	m := &metadata.SeriesMetadata{
		TVDBID:       &tvdbIDStr,
		Title:        result.Name,
		Overview:     result.Overview,
		PosterPath:   result.Image,
		FirstAirDate: result.FirstAirDate,
		Genres:       []string{},
	}

	// Extract IMDB ID if available
	if result.ExternalIDs.IMDB != "" {
		m.IMDBID = &result.ExternalIDs.IMDB
	}

	// Try to extract year from first air date (YYYY-MM-DD format)
	if result.FirstAirDate != "" {
		parts := strings.Split(result.FirstAirDate, "-")
		if len(parts) > 0 {
			if year, err := strconv.Atoi(parts[0]); err == nil {
				m.Seasons = year // Use Seasons field for year storage
			}
		}
	}

	return m
}
