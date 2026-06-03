package series

import "context"

// ProviderEpisode is a provider-agnostic episode record used to (re)segment a
// series' seasons and episodes from an external episode source whose numbering
// differs from TMDB (e.g. TVDB aired order for multi-cour anime).
type ProviderEpisode struct {
	SeasonNumber   int
	EpisodeNumber  int
	AbsoluteNumber int
	Runtime        int
	Title          string
	Overview       string
	AirDate        string
	StillPath      string
}

// EpisodeProvider supplies season/episode structure for series whose numbering
// differs between metadata sources. It is used for anime, where TMDB collapses
// multiple cours into one season but releases use the TVDB/scene S01/S02 split.
type EpisodeProvider interface {
	// ResolveSeriesID resolves the provider's numeric series ID from the title,
	// year and any known external IDs (e.g. "imdb", "tmdb"). It returns 0 when
	// no confident match is found.
	ResolveSeriesID(ctx context.Context, title string, year int, externalIDs map[string]string) (int, error)

	// SeriesEpisodes returns all episodes for the provider series ID in aired
	// (broadcast) order.
	SeriesEpisodes(ctx context.Context, providerSeriesID int) ([]ProviderEpisode, error)
}

// Option configures the series service.
type Option func(*service)

// WithEpisodeProvider configures an external episode provider used to segment
// anime series so their seasons match release numbering.
func WithEpisodeProvider(p EpisodeProvider) Option {
	return func(s *service) { s.episodeProvider = p }
}

// PostRefreshHook is invoked after a successful RefreshSeries. Because a refresh
// deletes and re-creates a series' episodes (assigning new episode IDs), any
// existing episode_file links are orphaned. The hook re-links on-disk files to
// the freshly created episodes (typically by triggering a series scan). It is
// best-effort: failures must not fail the refresh.
type PostRefreshHook func(ctx context.Context, seriesID string)

// WithPostRefreshHook configures a hook run after each successful refresh to
// re-link on-disk episode files to the re-created episodes.
func WithPostRefreshHook(fn PostRefreshHook) Option {
	return func(s *service) { s.postRefreshHook = fn }
}
