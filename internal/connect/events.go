package connect

import (
	"context"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
)

// SubscribeImportEvents refreshes configured media servers only after a completed
// import. The returned function removes the subscription during shutdown.
func SubscribeImportEvents(ctx context.Context, bus eventbus.Bus, svc Service) func() {
	return bus.Subscribe("imports.completed", func(_ context.Context, _ eventbus.Event) error {
		// Async delivery can outlive the request that published the import.
		notifyCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		return svc.NotifyAll(notifyCtx, "import")
	})
}
