// Command loom-updater is a root-only, fixed-service native updater.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ebenderooock/loom/internal/updater"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "loom-updater:", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// The launchd daemon bounds each transaction internally, not its lifetime.
	if len(os.Args) != 2 || (os.Args[1] != "daemon" && os.Args[1] != "service") {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
	}
	return updater.Main(ctx, os.Args[1:])
}
