package downloads

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
)

type capturingBusForMonitor struct {
	events []eventbus.Event
}

func (b *capturingBusForMonitor) Publish(ctx context.Context, ev eventbus.Event) error {
	b.events = append(b.events, ev)
	return nil
}

func (b *capturingBusForMonitor) Subscribe(topic string, h eventbus.Handler) func() {
	return func() {}
}

type fakeScheduler struct {
	// For testing, we don't actually schedule anything.
}

func (s *fakeScheduler) Register(name, schedule string, fn func(context.Context) error) error {
	return nil
}

type eventbusStub struct {
	capturingBusForMonitor
}

func newTestMonitor(t *testing.T, bus *eventbusStub) *Monitor {
	reg := NewRegistry()
	svc, err := NewService(ServiceOptions{
		Repository: &testRepository{},
		Registry:   reg,
		Logger:     slog.Default(),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	monitor, err := NewMonitor(MonitorOptions{
		Service:       svc,
		Bus:           bus,
		CheckInterval: 5 * time.Second,
		Logger:        slog.Default(),
		Clock:         &testClock{now: time.Now()},
	})
	if err != nil {
		t.Fatalf("NewMonitor: %v", err)
	}
	return monitor
}

func TestMonitorEmitsCompletionEvents(t *testing.T) {
	t.Parallel()

	bus := &eventbusStub{capturingBusForMonitor: capturingBusForMonitor{events: []eventbus.Event{}}}
	monitor := newTestMonitor(t, bus)

	// Simulate items returned from Status() call.
	items := []Item{
		{ID: "item-1", Title: "Completed Item 1", Status: StatusItemCompleted, Category: "movies"},
		{ID: "item-2", Title: "Downloading", Status: StatusItemDownloading},
		{ID: "item-3", Title: "Completed Item 2", Status: StatusItemCompleted, Category: "tv"},
	}

	// Run the monitor's completion emitter.
	monitor.emitCompletions(context.Background(), items)

	// Check that DownloadCompleted was emitted for the completed items.
	if len(bus.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(bus.events))
	}

	for i, ev := range bus.events {
		completed, ok := ev.(*DownloadCompletedEvent)
		if !ok {
			t.Fatalf("event %d is not *DownloadCompletedEvent", i)
		}
		if completed.Topic() != TopicDownloadCompleted {
			t.Errorf("event %d Topic = %q, want %q", i, completed.Topic(), TopicDownloadCompleted)
		}
	}

	// Verify the items were correct.
	ev1 := bus.events[0].(*DownloadCompletedEvent)
	if ev1.DownloadID != "item-1" {
		t.Errorf("event 0 DownloadID = %q, want item-1", ev1.DownloadID)
	}
	ev2 := bus.events[1].(*DownloadCompletedEvent)
	if ev2.DownloadID != "item-3" {
		t.Errorf("event 1 DownloadID = %q, want item-3", ev2.DownloadID)
	}
}

func TestMonitorDoesNotRepeatCompletions(t *testing.T) {
	t.Parallel()

	bus := &eventbusStub{capturingBusForMonitor: capturingBusForMonitor{events: []eventbus.Event{}}}
	monitor := newTestMonitor(t, bus)

	// First run: emit completions for items 1 and 2.
	items := []Item{
		{ID: "item-1", Title: "Completed Item 1", Status: StatusItemCompleted},
		{ID: "item-2", Title: "Completed Item 2", Status: StatusItemCompleted},
	}
	monitor.emitCompletions(context.Background(), items)
	if len(bus.events) != 2 {
		t.Fatalf("first run: expected 2 events, got %d", len(bus.events))
	}

	// Clear events for second run.
	bus.events = nil

	// Second run: same items. Should NOT emit duplicates.
	monitor.emitCompletions(context.Background(), items)
	if len(bus.events) != 0 {
		t.Fatalf("second run: expected 0 events (no duplicates), got %d", len(bus.events))
	}

	// Third run: one old, one new item.
	itemsWithNew := []Item{
		{ID: "item-1", Title: "Still Completed Item 1", Status: StatusItemCompleted},
		{ID: "item-3", Title: "Newly Completed", Status: StatusItemCompleted},
	}
	monitor.emitCompletions(context.Background(), itemsWithNew)
	if len(bus.events) != 1 {
		t.Fatalf("third run: expected 1 event (new only), got %d", len(bus.events))
	}
	ev := bus.events[0].(*DownloadCompletedEvent)
	if ev.DownloadID != "item-3" {
		t.Errorf("event DownloadID = %q, want item-3", ev.DownloadID)
	}
}

func TestMonitorRunSchedules(t *testing.T) {
	t.Parallel()

	bus := &eventbusStub{capturingBusForMonitor: capturingBusForMonitor{events: []eventbus.Event{}}}
	monitor := newTestMonitor(t, bus)

	// Monitor.Run should call Status() and process results.
	// We'll mock the status by registering a fake client that returns status.
	client := &fakeClientWithStatus{
		id: "test-client",
		items: []Item{
			{ID: "item-1", Title: "Completed", Status: StatusItemCompleted},
		},
	}
	if err := monitor.svc.registry.Register(client); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Run the monitor.
	err := monitor.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Check that a DownloadCompleted event was emitted.
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	ev := bus.events[0].(*DownloadCompletedEvent)
	if ev.Topic() != TopicDownloadCompleted {
		t.Errorf("Topic = %q, want %q", ev.Topic(), TopicDownloadCompleted)
	}
}

// fakeClientWithStatus returns fixed status results.
type fakeClientWithStatus struct {
	id    string
	items []Item
}

func (c *fakeClientWithStatus) ID() string         { return c.id }
func (c *fakeClientWithStatus) Name() string       { return "Fake Client with Status" }
func (c *fakeClientWithStatus) Kind() Kind         { return KindNull }
func (c *fakeClientWithStatus) Protocol() Protocol { return ProtocolTorrent }
func (c *fakeClientWithStatus) Add(ctx context.Context, req AddRequest) (AddResult, error) {
	return AddResult{}, nil
}
func (c *fakeClientWithStatus) Status(ctx context.Context, ids ...string) ([]Item, error) {
	return c.items, nil
}
func (c *fakeClientWithStatus) Pause(ctx context.Context, ids ...string) error  { return nil }
func (c *fakeClientWithStatus) Resume(ctx context.Context, ids ...string) error { return nil }
func (c *fakeClientWithStatus) Remove(ctx context.Context, ids []string, deleteFiles bool) error {
	return nil
}
func (c *fakeClientWithStatus) SetPriority(_ context.Context, _ Priority, _ ...string) error {
	return nil
}
func (c *fakeClientWithStatus) SetSpeedLimit(_ context.Context, _ int64, _ ...string) error {
	return nil
}
func (c *fakeClientWithStatus) ForceStart(_ context.Context, _ ...string) error    { return nil }
func (c *fakeClientWithStatus) Recheck(_ context.Context, _ ...string) error       { return nil }
func (c *fakeClientWithStatus) Reannounce(_ context.Context, _ ...string) error    { return nil }
func (c *fakeClientWithStatus) Categories(ctx context.Context) ([]Category, error) { return nil, nil }
func (c *fakeClientWithStatus) FreeSpace(ctx context.Context) (int64, error)       { return 0, nil }
func (c *fakeClientWithStatus) Test(ctx context.Context) error                     { return nil }
