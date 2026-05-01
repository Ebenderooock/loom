// Package main is the loom entrypoint.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/loomctl/loom/internal/buildinfo"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "loom: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usage(os.Stderr)
	}

	switch args[0] {
	case "serve":
		return cmdServe(ctx, args[1:])
	case "version", "--version", "-v":
		fmt.Println(buildinfo.String())
		return nil
	case "healthcheck":
		return cmdHealthcheck(ctx, args[1:])
	case "migrate":
		return cmdMigrate(ctx, args[1:])
	case "api-key":
		return cmdAPIKey(ctx, args[1:])
	case "user":
		return cmdUser(ctx, args[1:])
	case "help", "--help", "-h":
		return usage(os.Stdout)
	default:
		_ = usage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w *os.File) error {
	_, err := fmt.Fprint(w, `loom — unified media automation

Usage:
  loom <command> [flags]

Commands:
  serve         Run the loom server
  migrate       Migrate from radarr/sonarr/prowlarr databases
  api-key       Manage API keys (create|list|revoke)
  user          Manage users (create|passwd)
  healthcheck   Probe the local server (used by Docker HEALTHCHECK)
  version       Print version info
  help          Show this message

Run 'loom <command> --help' for command-specific flags.

Configuration is layered: defaults < /config/loom.yaml < environment
(LOOM_*) < command-line flags. See docs/configuration.md.
`)
	return err
}
