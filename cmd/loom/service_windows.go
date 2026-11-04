//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
)

// cmdService is deliberately separate from serve: SCM must start this command,
// while an interactive console continues to use signal-driven serve.
func cmdService(args []string) error {
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	configPath := fs.String("config", "", "absolute path to loom.yaml (required)")
	logPath := fs.String("log-file", "", "absolute append-only log file (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || !filepath.IsAbs(*configPath) || !filepath.IsAbs(*logPath) {
		return fmt.Errorf("service requires absolute --config and --log-file paths, with no positional arguments")
	}
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("detect SCM: %w", err)
	}
	if !isService {
		return fmt.Errorf("service must be launched by SCM; use 'serve' in a console")
	}
	// Unlike ordinary serve, fail on a missing/unreadable explicit config rather
	// than silently starting with working-directory defaults.
	if _, err := os.ReadFile(*configPath); err != nil {
		return fmt.Errorf("read service config: %w", err)
	}
	if err := os.Chdir(filepath.Dir(*configPath)); err != nil {
		return fmt.Errorf("set service working directory: %w", err)
	}
	logFile, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open service log: %w", err)
	}
	defer logFile.Close()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = logFile, logFile
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()

	return svc.Run("Loom", &loomService{
		serve: func(ctx context.Context) error {
			return cmdServe(ctx, []string{"--config", *configPath})
		},
	})
}

type loomService struct {
	serve func(context.Context) error
}

func (h *loomService) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.serve(ctx) }()

	// Running means the worker has been dispatched, not HTTP readiness. Errors
	// during initialization return a non-zero SCM exit code; healthcheck remains
	// the authoritative application readiness check.
	current := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	statuses <- current
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				fmt.Fprintf(os.Stderr, "loom service: %v\n", err)
				return true, 1
			}
			return false, 0
		case request, ok := <-requests:
			if !ok {
				requests = nil
				cancel()
				current = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: 120000}
				statuses <- current
				continue
			}
			switch request.Cmd {
			case svc.Interrogate:
				statuses <- current
			case svc.Stop, svc.Shutdown:
				if current.State != svc.StopPending {
					current = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: 120000}
					statuses <- current
					cancel()
				}
			}
		case <-ticker.C:
			if current.State == svc.StopPending {
				current.CheckPoint++
				statuses <- current
			}
		}
	}
}
