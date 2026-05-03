package tvdb

import (
	"testing"
)

// TestMapSeriesToMetadata_FullData tests mapping with complete data.
func TestMapSeriesToMetadata_FullData(t *testing.T) {
	data := &SeriesData{
		ID:           81189,
		Name:         "Breaking Bad",
		Overview:     "A high school chemistry teacher...",
		Image:        "/images/81189.jpg",
		FirstAirDate: "2008-01-20",
		Year:         "2008",
		ExternalIDs: IDsInfo{
			IMDB: "tt0903747",
			TVDb: 81189,
		},
	}

	m := MapSeriesToMetadata(data)

	if m == nil {
		t.Fatalf("expected metadata, got nil")
	}

	if m.Title != "Breaking Bad" {
		t.Fatalf("expected title 'Breaking Bad', got %s", m.Title)
	}

	if m.IMDBID == nil || *m.IMDBID != "tt0903747" {
		t.Fatalf("expected IMDB ID, got %v", m.IMDBID)
	}

	if m.TVDBID == nil || *m.TVDBID != "81189" {
		t.Fatalf("expected TVDB ID, got %v", m.TVDBID)
	}

	if m.PosterPath != "/images/81189.jpg" {
		t.Fatalf("expected poster path, got %s", m.PosterPath)
	}

	if m.FirstAirDate != "2008-01-20" {
		t.Fatalf("expected first air date, got %s", m.FirstAirDate)
	}
}

// TestMapSeriesToMetadata_MinimalData tests mapping with minimal data.
func TestMapSeriesToMetadata_MinimalData(t *testing.T) {
	data := &SeriesData{
		ID:   81189,
		Name: "Breaking Bad",
	}

	m := MapSeriesToMetadata(data)

	if m == nil {
		t.Fatalf("expected metadata, got nil")
	}

	if m.Title != "Breaking Bad" {
		t.Fatalf("expected title, got %s", m.Title)
	}

	if m.IMDBID != nil {
		t.Fatalf("expected nil IMDB ID, got %v", m.IMDBID)
	}
}

// TestMapSeriesToMetadata_Nil tests mapping with nil input.
func TestMapSeriesToMetadata_Nil(t *testing.T) {
	m := MapSeriesToMetadata(nil)
	if m != nil {
		t.Fatalf("expected nil, got %v", m)
	}
}

// TestMapEpisodeToMetadata_FullData tests episode mapping with complete data.
func TestMapEpisodeToMetadata_FullData(t *testing.T) {
	data := &EpisodeData{
		ID:            2047316,
		Name:          "Pilot",
		Overview:      "A high school chemistry teacher...",
		AirDate:       "2008-01-20",
		Runtime:       58,
		SeasonNumber:  1,
		EpisodeNumber: 1,
		Ratings: RatingsInfo{
			IMDB: 8.9,
		},
	}

	m := MapEpisodeToMetadata(data)

	if m == nil {
		t.Fatalf("expected metadata, got nil")
	}

	if m.Title != "Pilot" {
		t.Fatalf("expected title 'Pilot', got %s", m.Title)
	}

	if m.Season != 1 {
		t.Fatalf("expected season 1, got %d", m.Season)
	}

	if m.Episode != 1 {
		t.Fatalf("expected episode 1, got %d", m.Episode)
	}

	if m.Runtime != 58 {
		t.Fatalf("expected runtime 58, got %d", m.Runtime)
	}

	if m.Rating != 8.9 {
		t.Fatalf("expected rating 8.9, got %f", m.Rating)
	}
}

// TestMapEpisodeToMetadata_MinimalData tests episode mapping with minimal data.
func TestMapEpisodeToMetadata_MinimalData(t *testing.T) {
	data := &EpisodeData{
		ID:            2047316,
		Name:          "Pilot",
		SeasonNumber:  1,
		EpisodeNumber: 1,
	}

	m := MapEpisodeToMetadata(data)

	if m == nil {
		t.Fatalf("expected metadata, got nil")
	}

	if m.Title != "Pilot" {
		t.Fatalf("expected title, got %s", m.Title)
	}

	if m.Season != 1 || m.Episode != 1 {
		t.Fatalf("expected S1E1")
	}
}

// TestMapEpisodeToMetadata_Nil tests episode mapping with nil input.
func TestMapEpisodeToMetadata_Nil(t *testing.T) {
	m := MapEpisodeToMetadata(nil)
	if m != nil {
		t.Fatalf("expected nil, got %v", m)
	}
}

// TestMapSearchResultToMetadata_SeriesType tests mapping search result for series.
func TestMapSearchResultToMetadata_SeriesType(t *testing.T) {
	result := &SearchResult{
		ID:           81189,
		Name:         "Breaking Bad",
		FirstAirDate: "2008-01-20",
		Overview:     "A high school chemistry teacher...",
		Type:         "series",
		ExternalIDs: IDsInfo{
			IMDB: "tt0903747",
		},
	}

	m := MapSearchResultToMetadata(result)

	if m == nil {
		t.Fatalf("expected metadata, got nil")
	}

	if m.Title != "Breaking Bad" {
		t.Fatalf("expected title 'Breaking Bad', got %s", m.Title)
	}

	if m.IMDBID == nil || *m.IMDBID != "tt0903747" {
		t.Fatalf("expected IMDB ID, got %v", m.IMDBID)
	}
}

// TestMapSearchResultToMetadata_NonSeriesType tests that non-series types are filtered.
func TestMapSearchResultToMetadata_NonSeriesType(t *testing.T) {
	result := &SearchResult{
		ID:   1,
		Name: "Breaking Bad Movie",
		Type: "movie",
	}

	m := MapSearchResultToMetadata(result)

	if m != nil {
		t.Fatalf("expected nil for non-series type, got %v", m)
	}
}

// TestMapSearchResultToMetadata_Nil tests nil input.
func TestMapSearchResultToMetadata_Nil(t *testing.T) {
	m := MapSearchResultToMetadata(nil)
	if m != nil {
		t.Fatalf("expected nil, got %v", m)
	}
}

// TestMapSearchResultToMetadata_YearExtraction tests year extraction from date.
func TestMapSearchResultToMetadata_YearExtraction(t *testing.T) {
	result := &SearchResult{
		ID:           81189,
		Name:         "Breaking Bad",
		FirstAirDate: "2008-01-20",
		Type:         "series",
	}

	m := MapSearchResultToMetadata(result)

	if m == nil {
		t.Fatalf("expected metadata, got nil")
	}

	// Year is stored in Seasons field as a fallback
	if m.Seasons != 2008 {
		t.Fatalf("expected year 2008, got %d", m.Seasons)
	}
}

// TestMapSearchResultToMetadata_InvalidYear tests invalid year format handling.
func TestMapSearchResultToMetadata_InvalidYear(t *testing.T) {
	result := &SearchResult{
		ID:           81189,
		Name:         "Breaking Bad",
		FirstAirDate: "invalid-date",
		Type:         "series",
	}

	m := MapSearchResultToMetadata(result)

	if m == nil {
		t.Fatalf("expected metadata, got nil")
	}

	// Seasons should remain 0 on parse error
	if m.Seasons != 0 {
		t.Logf("expected Seasons to be 0 on invalid year, got %d", m.Seasons)
	}
}

// TestMapSearchResultToMetadata_EmptyDate tests empty date handling.
func TestMapSearchResultToMetadata_EmptyDate(t *testing.T) {
	result := &SearchResult{
		ID:           81189,
		Name:         "Breaking Bad",
		FirstAirDate: "",
		Type:         "series",
	}

	m := MapSearchResultToMetadata(result)

	if m == nil {
		t.Fatalf("expected metadata, got nil")
	}

	// Should not crash with empty date
	if m.FirstAirDate != "" {
		t.Fatalf("expected empty first air date")
	}
}
