package downloads_test

import (
	"context"
	"errors"
	"testing"

	"github.com/loomctl/loom/internal/downloads"
)

func TestHealthCheckerNoClients(t *testing.T) {
	t.Parallel()
	svc := newServiceForTest(t)
	hc := downloads.NewHealthChecker(svc, 0, 0)
	if err := hc.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestHealthCheckerSweepRecordsOutcomes(t *testing.T) {
	t.Parallel()
	svc := newServiceForTest(t)
	ctx := context.Background()

	// Use the in-package null kind to seed two clients.
	for _, id := range []string{"a", "b"} {
		if _, err := svc.Create(ctx, downloads.Definition{
			ID: id, Name: id, Kind: downloads.KindNull, Protocol: downloads.ProtocolTorrent, Enabled: true,
		}); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}

	hc := downloads.NewHealthChecker(svc, 4, 0)
	if err := hc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, id := range []string{"a", "b"} {
		dh, err := svc.GetWithHealth(ctx, id)
		if err != nil {
			t.Fatalf("GetWithHealth %s: %v", id, err)
		}
		if dh.Health == nil || dh.Health.Status != downloads.StatusOK {
			t.Fatalf("expected ok health for %s: %#v", id, dh.Health)
		}
	}
}

func TestHealthCheckerNilService(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on nil service")
		}
	}()
	_ = downloads.NewHealthChecker(nil, 1, 0)
}

func TestHealthCheckerCanceledContext(t *testing.T) {
	t.Parallel()
	svc := newServiceForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	hc := downloads.NewHealthChecker(svc, 1, 0)
	// No clients yet, returns nil even with canceled ctx.
	if err := hc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: %v", err)
	}
}
