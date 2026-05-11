package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/ebenderooock/loom/internal/auditlog"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/metadata/tmdb"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/notifications"
	"github.com/ebenderooock/loom/internal/organizer"
	"github.com/ebenderooock/loom/internal/scanner"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/storage"
)

// defaultTMDBKey is a bundled application-level TMDB API key.
// Override via LOOM_TMDB_API_KEY env var or config.
const defaultTMDBKey = "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiI3NzU0NWI2ODU0ZjIzNGZjYjRhYzdlZTQzM2FjMTc4MyIsIm5iZiI6MTQyNDA4OTIyNi45ODgsInN1YiI6IjU0ZTFlMDhhOTI1MTQxMmM4ZTAwMTM2ZiIsInNjb3BlcyI6WyJhcGlfcmVhZCJdLCJ2ZXJzaW9uIjoxfQ.sS6ImS7Y3HZKNLF6z8G_G8kVafIyYmZHKbOUtSydiMI"

// buildMoviesService constructs the movies.Service backed by the storage
// engine in cfg and returns the wired service.
func buildMoviesService(ctx context.Context, cfg *config.Config, db storage.DB, logger *slog.Logger, bus eventbus.Bus) (movies.Service, error) {
	repo := movies.NewRepository(db.DB())

	// Build metadata service with TMDB provider
	apiKey := os.Getenv("LOOM_TMDB_API_KEY")
	if apiKey == "" {
		apiKey = defaultTMDBKey
	}

	tmdbClient := tmdb.NewClient(tmdb.Config{
		APIKey: apiKey,
	})
	tmdbProvider := tmdb.NewProvider(tmdbClient)

	metadataRepo := metadata.NewSQLiteRepository(db.DB())
	metadataSvc := metadata.NewService(metadataRepo, []metadata.MetadataProvider{tmdbProvider})

	svc := movies.NewService(repo, movies.WithMetadata(metadataSvc), movies.WithCredits(tmdbClient), movies.WithEventBus(bus))

	// Seed default quality definitions and profiles on first run
	movies.SeedDefaults(ctx, svc)

	return svc, nil
}

// buildScanner constructs the library scanner backed by the movies service.
func buildScanner(moviesSvc movies.Service, cfg *config.Config, auditLogger *auditlog.Logger, logger *slog.Logger) *scanner.Scanner {
	apiKey := os.Getenv("LOOM_TMDB_API_KEY")
	if apiKey == "" {
		apiKey = defaultTMDBKey
	}

	tmdbClient := tmdb.NewClient(tmdb.Config{APIKey: apiKey})
	tmdbProvider := tmdb.NewProvider(tmdbClient)

	metaSearcher := &metadataSearcherAdapter{provider: tmdbProvider}

	return scanner.New(moviesSvc, metaSearcher, logger, scanner.WithAuditLogger(auditLogger))
}

// metadataSearcherAdapter wraps a metadata.MetadataProvider to implement scanner.MetadataSearcher.
type metadataSearcherAdapter struct {
	provider metadata.MetadataProvider
}

func (a *metadataSearcherAdapter) FindMovieByQuery(ctx context.Context, query string, year int) ([]*metadata.MovieMetadata, error) {
	return a.provider.FindMovie(ctx, query, year, nil)
}

// buildOrganizer constructs the file organizer backed by movies service and DB.
func buildOrganizer(moviesSvc movies.Service, libStore *libraries.Store, db storage.DB, logger *slog.Logger) *organizer.Organizer {
	repo := movies.NewRepository(db.DB())
	configStore := organizer.NewSQLiteConfigStore(db.DB())

	return organizer.New(
		&organizer.MovieServiceAdapter{Svc: moviesSvc, LibStore: libStore},
		&organizer.RepoFileUpdater{Repo: repo},
		configStore,
		logger,
	)
}

// buildSeriesService constructs the TV series service.
func buildSeriesService(db storage.DB) series.Service {
	apiKey := os.Getenv("LOOM_TMDB_API_KEY")
	if apiKey == "" {
		apiKey = defaultTMDBKey
	}
	repo := series.NewRepository(db.DB())
	return series.NewService(repo, apiKey)
}

// buildSeriesScanner constructs the series scanner backed by the series service.
func buildSeriesScanner(seriesSvc series.Service, logger *slog.Logger) *scanner.SeriesScanner {
	return scanner.NewSeriesScanner(seriesSvc, logger)
}

// buildNotificationsService constructs the notifications service.
func buildNotificationsService(db storage.DB) notifications.Service {
	return notifications.NewService(db.DB())
}

// buildTMDBClient constructs a TMDB API client using the configured key.
func buildTMDBClient() *tmdb.Client {
	apiKey := os.Getenv("LOOM_TMDB_API_KEY")
	if apiKey == "" {
		apiKey = defaultTMDBKey
	}
	return tmdb.NewClient(tmdb.Config{APIKey: apiKey})
}
