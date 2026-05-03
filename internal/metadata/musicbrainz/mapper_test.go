package musicbrainz

import (
	"testing"
)

func TestMapArtist_Success(t *testing.T) {
	resp := &ArtistResponse{
		ID:             "artist-1",
		Name:           "The Beatles",
		Disambiguation: "British rock band",
		Area: &AreaResponse{
			ID:   "area-1",
			Name: "United Kingdom",
		},
	}

	result := MapArtist(resp)

	if result.MBID != "artist-1" {
		t.Errorf("expected MBID 'artist-1', got %q", result.MBID)
	}
	if result.Name != "The Beatles" {
		t.Errorf("expected name 'The Beatles', got %q", result.Name)
	}
	if result.Disambiguation != "British rock band" {
		t.Errorf("expected disambiguation, got %q", result.Disambiguation)
	}
	if result.Area != "United Kingdom" {
		t.Errorf("expected area 'United Kingdom', got %q", result.Area)
	}
}

func TestMapArtist_WithCountry(t *testing.T) {
	resp := &ArtistResponse{
		ID:      "artist-1",
		Name:    "Artist Name",
		Country: "US",
	}

	result := MapArtist(resp)

	if result.Area != "US" {
		t.Errorf("expected area 'US', got %q", result.Area)
	}
}

func TestMapArtist_Nil(t *testing.T) {
	result := MapArtist(nil)

	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestMapRelease_Success(t *testing.T) {
	resp := &ReleaseResponse{
		ID:    "release-1",
		Title: "Abbey Road",
		Date:  "1969-09-26",
		Artists: []ArtistResponse{
			{ID: "artist-1", Name: "The Beatles"},
		},
		Media: []MediaResponse{
			{
				Position: "1",
				Tracks: []TrackResponse{
					{Title: "Come Together"},
					{Title: "Something"},
				},
			},
		},
	}

	result := MapRelease(resp)

	if result.MBID != "release-1" {
		t.Errorf("expected MBID 'release-1', got %q", result.MBID)
	}
	if result.Title != "Abbey Road" {
		t.Errorf("expected title 'Abbey Road', got %q", result.Title)
	}
	if result.Year != 1969 {
		t.Errorf("expected year 1969, got %d", result.Year)
	}
	if len(result.Artists) != 1 || result.Artists[0] != "The Beatles" {
		t.Errorf("expected artists ['The Beatles'], got %v", result.Artists)
	}
	if len(result.Tracks) != 2 {
		t.Errorf("expected 2 tracks, got %d", len(result.Tracks))
	}
}

func TestMapRelease_YearFromDate(t *testing.T) {
	resp := &ReleaseResponse{
		ID:    "release-1",
		Title: "Album",
		Date:  "1975-03-15",
	}

	result := MapRelease(resp)

	if result.Year != 1975 {
		t.Errorf("expected year 1975, got %d", result.Year)
	}
}

func TestMapRelease_DirectYear(t *testing.T) {
	resp := &ReleaseResponse{
		ID:    "release-1",
		Title: "Album",
		Year:  2000,
		Date:  "2000-01-01",
	}

	result := MapRelease(resp)

	if result.Year != 2000 {
		t.Errorf("expected year 2000, got %d", result.Year)
	}
}

func TestMapRelease_Nil(t *testing.T) {
	result := MapRelease(nil)

	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestMapRecording_Success(t *testing.T) {
	resp := &RecordingResponse{
		ID:     "recording-1",
		Title:  "Imagine",
		Length: 183000,
		Artists: []ArtistResponse{
			{ID: "artist-1", Name: "John Lennon"},
		},
	}

	result := MapRecording(resp)

	if result.MBID != "recording-1" {
		t.Errorf("expected MBID 'recording-1', got %q", result.MBID)
	}
	if result.Title != "Imagine" {
		t.Errorf("expected title 'Imagine', got %q", result.Title)
	}
	if result.Duration != 183000 {
		t.Errorf("expected duration 183000, got %d", result.Duration)
	}
	if len(result.Artists) != 1 || result.Artists[0] != "John Lennon" {
		t.Errorf("expected artists ['John Lennon'], got %v", result.Artists)
	}
}

func TestMapRecording_MultipleArtists(t *testing.T) {
	resp := &RecordingResponse{
		ID:    "recording-1",
		Title: "Song",
		Artists: []ArtistResponse{
			{ID: "artist-1", Name: "Artist 1"},
			{ID: "artist-2", Name: "Artist 2"},
			{ID: "artist-3", Name: "Artist 3"},
		},
	}

	result := MapRecording(resp)

	if len(result.Artists) != 3 {
		t.Errorf("expected 3 artists, got %d", len(result.Artists))
	}
}

func TestMapRecording_Nil(t *testing.T) {
	result := MapRecording(nil)

	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestExtractArtistNames(t *testing.T) {
	artists := []ArtistResponse{
		{Name: "Artist 1"},
		{Name: "Artist 2"},
		{Name: ""},
		{Name: "Artist 3"},
	}

	result := extractArtistNames(artists)

	if len(result) != 3 {
		t.Errorf("expected 3 names, got %d", len(result))
	}

	expected := []string{"Artist 1", "Artist 2", "Artist 3"}
	for i, name := range expected {
		if result[i] != name {
			t.Errorf("expected %q at index %d, got %q", name, i, result[i])
		}
	}
}

func TestExtractTrackTitles(t *testing.T) {
	media := []MediaResponse{
		{
			Position: "1",
			Tracks: []TrackResponse{
				{Title: "Track 1"},
				{Title: "Track 2"},
			},
		},
		{
			Position: "2",
			Tracks: []TrackResponse{
				{Title: "Track 3"},
			},
		},
	}

	result := extractTrackTitles(media)

	if len(result) != 3 {
		t.Errorf("expected 3 titles, got %d", len(result))
	}

	expected := []string{"Track 1", "Track 2", "Track 3"}
	for i, title := range expected {
		if result[i] != title {
			t.Errorf("expected %q at index %d, got %q", title, i, result[i])
		}
	}
}

func TestExtractCoverartURL(t *testing.T) {
	relations := []RelationResponse{
		{
			Type:       "cover art",
			Target:     "https://example.com/image.jpg",
			TargetType: "url",
		},
	}

	result := extractCoverartURL(relations)

	if result != "https://example.com/image.jpg" {
		t.Errorf("expected URL, got %q", result)
	}
}

func TestExtractCoverartURL_NoURL(t *testing.T) {
	relations := []RelationResponse{
		{Type: "other relationship"},
	}

	result := extractCoverartURL(relations)

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}
