// Command loom is the entrypoint binary for the Loom server.
//
// Subcommands:
//
//	serve         run the HTTP server (the default deployment mode)
//	migrate       apply schema migrations / import from arr DBs (Phase 8)
//	healthcheck   probe the local server (used by the Docker HEALTHCHECK)
//	version       print build metadata
//
// Configuration is layered: built-in defaults < $LOOM_CONFIG_DIR/loom.yaml
// (or --config) < environment variables (LOOM_*) < command-line flags.
// See docs/configuration.md for the exhaustive key reference and
// docs/development.md for the local run loop.
package main
