package tmdb

import (
	"context"
	"strconv"

	"github.com/loomctl/loom/internal/metadata"
)

// Provider implements the metadata.MetadataProvider interface for TMDb.
type Provider struct {
	client *Client
}

// NewProvider builds a new TMDb metadata provider.
func NewProvider(client *Client) *Provider {
	return &Provider{client: client}
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "tmdb"
}

// FindMovie implements metadata.MetadataProvider.FindMovie.
// It prioritizes TMDB ID lookup, then falls back to search by title+year.
func (p *Provider) FindMovie(ctx context.Context, title string, year int, externalIDs map[string]string) ([]*metadata.MovieMetadata, error) {
	// If we have a TMDB ID, use direct lookup
	if tmdbID, ok := externalIDs["tmdb"]; ok {
		id, err := strconv.Atoi(tmdbID)
		if err == nil {
			m, err := p.client.GetMovie(ctx, id)
			if err == nil && m != nil {
				return []*metadata.MovieMetadata{m}, nil
			}
		}
	}

	// Fall back to search by title and year
	if title != "" {
		results, err := p.client.SearchMovie(ctx, title, year)
		if err == nil && len(results) > 0 {
			return results, nil
		}
	}

	return nil, nil
}

// FindSeries implements metadata.MetadataProvider.FindSeries.
// It prioritizes TMDB ID lookup, then falls back to search by title.
func (p *Provider) FindSeries(ctx context.Context, title string, externalIDs map[string]string) ([]*metadata.SeriesMetadata, error) {
	// If we have a TMDB ID, use direct lookup
	if tmdbID, ok := externalIDs["tmdb"]; ok {
		id, err := strconv.Atoi(tmdbID)
		if err == nil {
			s, err := p.client.GetTV(ctx, id)
			if err == nil && s != nil {
				return []*metadata.SeriesMetadata{s}, nil
			}
		}
	}

	// Fall back to search by title
	if title != "" {
		results, err := p.client.SearchTV(ctx, title, 0)
		if err == nil && len(results) > 0 {
			return results, nil
		}
	}

	return nil, nil
}

// FindEpisode implements metadata.MetadataProvider.FindEpisode.
// seriesID is expected to be a TMDB TV ID.
func (p *Provider) FindEpisode(ctx context.Context, seriesID string, season int, episode int) (*metadata.EpisodeMetadata, error) {
	tvID, err := strconv.Atoi(seriesID)
	if err != nil {
		// seriesID is not a TMDB ID; can't look up episode
		return nil, nil
	}

	ep, err := p.client.GetEpisode(ctx, tvID, season, episode)
	if err != nil {
		// If 404, return nil (not found) rather than error
		if clientErr, ok := err.(*ClientError); ok && clientErr.Code == ErrCodeNotFound {
			return nil, nil
		}
		return nil, err
	}

	return ep, nil
}
