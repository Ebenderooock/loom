package main

import (
	"context"
	"log/slog"

	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/movies"
	"github.com/loomctl/loom/internal/storage"
)

// buildMoviesService constructs the movies.Service backed by the storage
// engine in cfg and returns the wired service.
func buildMoviesService(ctx context.Context, cfg *config.Config, db storage.DB, logger *slog.Logger) (movies.Service, error) {
	repo := movies.NewRepository(db.DB())
	return movies.NewService(repo), nil
}

