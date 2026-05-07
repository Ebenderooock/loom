package main

import (
	"log/slog"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/indexers/newznabserver"
)

// wireAggregator constructs the Newznab/Torznab aggregator server
// that exposes all configured indexers over a standard search API.
func wireAggregator(indexerSvc *indexers.Service, authSvc *auth.Service, logger *slog.Logger) (*newznabserver.Server, error) {
	return newznabserver.NewServer(newznabserver.Options{
		Search:    indexerSvc,
		Auth:      authSvc,
		Logger:    logger,
		Title:     "Loom",
		Strapline: "Loom Newznab/Torznab aggregator",
	})
}
