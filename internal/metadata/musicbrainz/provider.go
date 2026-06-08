package musicbrainz

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/metadata"
)

// coverArtArchiveBase is the free Cover Art Archive endpoint. Requesting
// /release-group/{mbid}/front redirects to the front cover image.
const coverArtArchiveBase = "https://coverartarchive.org"

// Provider implements metadata.MusicMetadataProvider backed by MusicBrainz
// (with cover art from the Cover Art Archive).
type Provider struct {
	client *Client
}

// NewProvider builds a MusicBrainz music metadata provider.
func NewProvider(client *Client) *Provider {
	if client == nil {
		client = NewClient(DefaultConfig())
	}
	return &Provider{client: client}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "musicbrainz" }

// SearchArtist implements metadata.MusicMetadataProvider.
func (p *Provider) SearchArtist(ctx context.Context, query string, limit int) ([]*metadata.ArtistMetadata, error) {
	if limit <= 0 {
		limit = 10
	}
	results, err := p.client.SearchArtist(ctx, query, 0, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*metadata.ArtistMetadata, 0, len(results))
	for _, a := range results {
		if a == nil {
			continue
		}
		out = append(out, &metadata.ArtistMetadata{
			MBID:           a.MBID,
			Name:           a.Name,
			Disambiguation: a.Disambiguation,
			Country:        a.Area,
		})
	}
	return out, nil
}

// GetArtist implements metadata.MusicMetadataProvider.
func (p *Provider) GetArtist(ctx context.Context, mbid string) (*metadata.ArtistMetadata, error) {
	a, err := p.client.GetArtist(ctx, mbid)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, NewNotFoundError("artist not found")
	}
	return &metadata.ArtistMetadata{
		MBID:           a.MBID,
		Name:           a.Name,
		Disambiguation: a.Disambiguation,
		Country:        a.Area,
	}, nil
}

// GetArtistAlbums implements metadata.MusicMetadataProvider. It pages through
// all release-groups for the artist.
func (p *Provider) GetArtistAlbums(ctx context.Context, artistMBID string) ([]*metadata.AlbumMetadata, error) {
	const pageSize = 100
	var albums []*metadata.AlbumMetadata
	offset := 0
	for {
		page, total, err := p.client.GetArtistReleaseGroups(ctx, artistMBID, offset, pageSize)
		if err != nil {
			return nil, err
		}
		for i := range page {
			albums = append(albums, mapReleaseGroup(&page[i]))
		}
		offset += len(page)
		if len(page) == 0 || offset >= total {
			break
		}
	}
	return albums, nil
}

// GetAlbum implements metadata.MusicMetadataProvider.
func (p *Provider) GetAlbum(ctx context.Context, releaseGroupMBID string) (*metadata.AlbumMetadata, []*metadata.AlbumReleaseMetadata, error) {
	rg, err := p.client.GetReleaseGroup(ctx, releaseGroupMBID)
	if err != nil {
		return nil, nil, err
	}
	album := mapReleaseGroup(rg)
	releases := make([]*metadata.AlbumReleaseMetadata, 0, len(rg.Releases))
	for i := range rg.Releases {
		releases = append(releases, mapReleaseSummary(&rg.Releases[i]))
	}
	return album, releases, nil
}

// GetAlbumRelease implements metadata.MusicMetadataProvider.
func (p *Provider) GetAlbumRelease(ctx context.Context, releaseMBID string) (*metadata.AlbumReleaseMetadata, error) {
	r, err := p.client.GetReleaseRaw(ctx, releaseMBID)
	if err != nil {
		return nil, err
	}
	return mapReleaseWithTracks(r), nil
}

// --- mapping helpers ---

func mapReleaseGroup(rg *ReleaseGroupFull) *metadata.AlbumMetadata {
	if rg == nil {
		return nil
	}
	artistMBID, artistName := firstArtist(rg.Artists)
	return &metadata.AlbumMetadata{
		MBID:           rg.ID,
		Title:          rg.Title,
		ArtistMBID:     artistMBID,
		ArtistName:     artistName,
		Type:           rg.PrimaryType,
		SecondaryTypes: rg.SecondaryTypes,
		ReleaseDate:    rg.FirstReleaseDate,
		Genres:         mapGenres(rg.Genres),
		CoverArtURL:    fmt.Sprintf("%s/release-group/%s/front", coverArtArchiveBase, rg.ID),
	}
}

func mapReleaseSummary(r *ReleaseResponse) *metadata.AlbumReleaseMetadata {
	if r == nil {
		return nil
	}
	return &metadata.AlbumReleaseMetadata{
		MBID:        r.ID,
		Title:       r.Title,
		Status:      r.Status,
		ReleaseDate: r.Date,
		Format:      firstFormat(r.Media),
		MediaCount:  len(r.Media),
		TrackCount:  countTracks(r.Media),
	}
}

func mapReleaseWithTracks(r *ReleaseResponse) *metadata.AlbumReleaseMetadata {
	if r == nil {
		return nil
	}
	out := mapReleaseSummary(r)
	for discIdx, m := range r.Media {
		disc := parsePosition(m.Position, discIdx+1)
		for trackIdx, t := range m.Tracks {
			track := metadata.TrackMetadata{
				TrackID:     t.ID,
				Title:       t.Title,
				TrackNumber: parsePosition(t.Number, parsePosition(t.Position, trackIdx+1)),
				DiscNumber:  disc,
				DurationMs:  t.Length,
			}
			if t.Recording != nil {
				track.MBID = t.Recording.ID
				if track.Title == "" {
					track.Title = t.Recording.Title
				}
				if track.DurationMs == 0 {
					track.DurationMs = t.Recording.Length
				}
			}
			out.Tracks = append(out.Tracks, track)
		}
	}
	return out
}

func firstArtist(credits []ArtistResponse) (mbid, name string) {
	if len(credits) == 0 {
		return "", ""
	}
	return credits[0].ID, credits[0].Name
}

func mapGenres(genres []GenreResponse) []string {
	if len(genres) == 0 {
		return nil
	}
	out := make([]string, 0, len(genres))
	for _, g := range genres {
		if g.Name != "" {
			out = append(out, g.Name)
		}
	}
	return out
}

func firstFormat(media []MediaResponse) string {
	for _, m := range media {
		if m.Format != "" {
			return m.Format
		}
	}
	return ""
}

func countTracks(media []MediaResponse) int {
	n := 0
	for _, m := range media {
		n += len(m.Tracks)
	}
	return n
}

// parsePosition parses a MusicBrainz position/number string (e.g. "1", "A1")
// into an int, returning fallback when it cannot be parsed cleanly.
func parsePosition(s string, fallback int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	// Vinyl-style numbering like "A1": strip leading non-digits.
	digits := strings.TrimLeftFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	if n, err := strconv.Atoi(digits); err == nil {
		return n
	}
	return fallback
}
