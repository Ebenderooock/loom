package musicbrainz

import (
	"strconv"
	"strings"
)

// MapArtist converts an ArtistResponse to ArtistMetadata.
func MapArtist(resp *ArtistResponse) *ArtistMetadata {
	if resp == nil {
		return nil
	}

	var area string
	if resp.Area != nil {
		area = resp.Area.Name
	} else if resp.Country != "" {
		area = resp.Country
	}

	return &ArtistMetadata{
		MBID:           resp.ID,
		Name:           resp.Name,
		Disambiguation: resp.Disambiguation,
		Area:           area,
	}
}

// MapRelease converts a ReleaseResponse to ReleaseMetadata.
func MapRelease(resp *ReleaseResponse) *ReleaseMetadata {
	if resp == nil {
		return nil
	}

	// Extract year from date string (ISO 8601: YYYY-MM-DD) if year not directly provided
	year := resp.Year
	if year == 0 && resp.Date != "" {
		// Parse YYYY from date
		parts := strings.Split(resp.Date, "-")
		if len(parts) > 0 {
			if y, err := strconv.Atoi(parts[0]); err == nil {
				year = y
			}
		}
	}

	// Extract artist names from artist-credit
	artists := extractArtistNames(resp.Artists)

	// Extract track titles from media
	tracks := extractTrackTitles(resp.Media)

	// Extract cover art URL
	coverartURL := extractCoverartURL(resp.Relations)

	return &ReleaseMetadata{
		MBID:         resp.ID,
		Title:        resp.Title,
		Year:         year,
		Artists:      artists,
		Tracks:       tracks,
		CoverartURL:  coverartURL,
	}
}

// MapRecording converts a RecordingResponse to RecordingMetadata.
func MapRecording(resp *RecordingResponse) *RecordingMetadata {
	if resp == nil {
		return nil
	}

	artists := extractArtistNames(resp.Artists)

	return &RecordingMetadata{
		MBID:     resp.ID,
		Title:    resp.Title,
		Duration: resp.Length,
		Artists:  artists,
	}
}

// extractArtistNames extracts artist names from an artist-credit array.
func extractArtistNames(artists []ArtistResponse) []string {
	var names []string
	for _, artist := range artists {
		if artist.Name != "" {
			names = append(names, artist.Name)
		}
	}
	return names
}

// extractTrackTitles extracts track titles from media array.
func extractTrackTitles(media []MediaResponse) []string {
	var titles []string
	for _, m := range media {
		for _, track := range m.Tracks {
			if track.Title != "" {
				titles = append(titles, track.Title)
			}
		}
	}
	return titles
}

// extractCoverartURL extracts cover art URL from relations.
// MusicBrainz stores cover art URLs in relations with type "cover art".
func extractCoverartURL(relations []RelationResponse) string {
	for _, rel := range relations {
		if rel.Type == "cover art" || rel.TargetType == "url" {
			if rel.Target != "" {
				return rel.Target
			}
		}
	}
	return ""
}
