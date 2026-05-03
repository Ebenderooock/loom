// Package metadata provides an abstraction layer for movie, series, and
// episode metadata sourced from external providers (TMDB, TVDB, MusicBrainz).
//
// # Architecture
//
// The package follows a tiered lookup strategy:
//
// 1. In-process cache (memory, TTL-based)
// 2. SQL database (SQLite or Postgres)
// 3. External providers (TMDB, TVDB, etc.)
//
// Metadata is immutable once cached; updates require explicit re-fetch via
// provider API calls (not cache invalidation).
//
// # Types
//
// MovieMetadata, SeriesMetadata, and EpisodeMetadata represent the data
// model. Each can be keyed by multiple external IDs (TMDB, IMDB, TVDB).
//
// MetadataProvider is the interface implemented by provider packages
// (e.g. internal/providers/tmdb, internal/providers/tvdb). Providers are
// queried sequentially with per-provider timeout (3s each, 10s total).
//
// # Service
//
// The Service struct orchestrates lookups:
//
//	svc := NewService(repo, []MetadataProvider{tmdbProvider, tvdbProvider})
//	movie, err := svc.FindMovie(ctx, SearchMovieParams{Title: "Inception", Year: 2010})
//	series, err := svc.FindSeries(ctx, SearchSeriesParams{Title: "Breaking Bad"})
//	episode, err := svc.FindEpisode(ctx, tvdbSeriesID, 5, 14)
//
// FindMovie and FindSeries return (nil, nil) if no providers yield a result
// or all timeout. Partial results are acceptable (e.g. if provider 1
// succeeds but provider 2 times out, the result from provider 1 is returned).
//
// # Cache TTLs
//
// - Search results (title+year lookup): 30 minutes
// - Full details (external ID lookup): 7 days
//
// Expired entries are cleaned up every 5 minutes by a background goroutine.
//
// # Database
//
// The Repository interface abstracts SQL access; concrete implementations
// dispatch to sqlc-generated code for SQLite or Postgres. Three tables:
//
//	- metadata_movies(id, tmdb_id, imdb_id, tvdb_id, title, year, ...)
//	- metadata_series(id, tmdb_id, imdb_id, tvdb_id, title, ...)
//	- metadata_episodes(id, series_id, season, episode, tvdb_id, ...)
//
// See migration 0011 for schema.
//
// # Concurrency
//
// The Service is safe for concurrent use. The in-process cache uses
// sync.Mutex; the Repository delegates to the database driver (safe).
//
// # Deferred to Phase 5+
//
// - Redis caching layer for horizontal scale
// - Cache invalidation strategies (webhooks, scheduled refreshes)
// - Bulk metadata import from providers
// - Provider fallback strategies (e.g. try TVDB if TMDB rate-limited)
// - Metadata scoring (pick best result from multiple providers)
//
package metadata
