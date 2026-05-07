package downloads

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/metadata"
)

type testClock struct {
	now time.Time
}

func (c *testClock) Now() time.Time { return c.now }

type testEvent struct {
	topic string
	data  eventbus.Event
}

type capturingBus struct {
	events []testEvent
}

func (b *capturingBus) Publish(ctx context.Context, ev eventbus.Event) error {
	b.events = append(b.events, testEvent{topic: ev.Topic(), data: ev})
	return nil
}

func (b *capturingBus) Subscribe(topic string, h eventbus.Handler) func() {
	return func() {}
}

func newTestRouter(t *testing.T, bus eventbus.Bus) (*Router, *testClock) {
	reg := NewRegistry()
	svc, err := NewService(ServiceOptions{
		Repository: &testRepository{},
		Registry:   reg,
		Logger:     slog.Default(),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// For testing, we can pass nil for metadata router since enrichment is non-blocking.
	// In real use, this would be initialized with actual providers.
	var metadataRouter *metadata.Router

	clock := &testClock{now: time.Now()}
	router := NewRouter(svc, metadataRouter, bus, slog.Default(), clock)
	return router, clock
}

func TestRouterQueuedOnSuccess(t *testing.T) {
	t.Parallel()

	bus := &capturingBus{}
	router, _ := newTestRouter(t, bus)
	defer router.Close()

	// Register a test client.
	client := &fakeClient{id: "test-client"}
	if err := router.svc.registry.Register(client); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Publish an indexer result.
	result := &indexers.Result{
		IndexerID: "testdex",
		GUID:      "test-guid-1",
		Title:     "Big Buck Bunny",
		Link:      "http://example.com/bbb.torrent",
		Infohash:  "1234567890abcdef1234567890abcdef12345678",
		Seeders:   ptrInt(10),
		Peers:     ptrInt(20),
		MagnetURI: "",
	}

	// Invoke the router's handler directly with wrapped event.
	err := router.handleIndexerResult(context.Background(), &IndexerResultEvent{Result: result})
	if err != nil {
		t.Fatalf("handleIndexerResult: %v", err)
	}

	// Check that DownloadQueued was emitted.
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	ev := bus.events[0].data.(*DownloadQueuedEvent)
	if ev.ClientID != "test-client" {
		t.Errorf("ClientID = %q, want test-client", ev.ClientID)
	}
	if ev.OriginResultID != "test-guid-1" {
		t.Errorf("OriginResultID = %q, want test-guid-1", ev.OriginResultID)
	}
	if ev.Topic() != TopicDownloadQueued {
		t.Errorf("Topic = %q, want %q", ev.Topic(), TopicDownloadQueued)
	}
}

func TestRouterFailsOnAddError(t *testing.T) {
	t.Parallel()

	bus := &capturingBus{}
	router, _ := newTestRouter(t, bus)
	defer router.Close()

	// Register a failing client.
	client := &fakeClient{
		id:    "failing-client",
		addErr: fmt.Errorf("disk full"),
	}
	if err := router.svc.registry.Register(client); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Publish an indexer result.
	result := &indexers.Result{
		GUID:      "test-guid-2",
		Title:     "Test Title",
		Seeders:   ptrInt(5),
		MagnetURI: "magnet:?xt=urn:btih:abcd",
	}

	// Invoke the router's handler.
	err := router.handleIndexerResult(context.Background(), &IndexerResultEvent{Result: result})
	if err != nil {
		t.Fatalf("handleIndexerResult: %v", err)
	}

	// Check that DownloadFailed was emitted.
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	ev := bus.events[0].data.(*DownloadFailureEvent)
	if ev.Topic() != TopicDownloadFailed {
		t.Errorf("Topic = %q, want %q", ev.Topic(), TopicDownloadFailed)
	}
	if ev.OriginResultID != "test-guid-2" {
		t.Errorf("OriginResultID = %q, want test-guid-2", ev.OriginResultID)
	}
}

func TestRouterNoClientsConfigured(t *testing.T) {
	t.Parallel()

	bus := &capturingBus{}
	router, _ := newTestRouter(t, bus)
	defer router.Close()

	// Don't register any clients.

	// Publish an indexer result.
	result := &indexers.Result{
		GUID:      "test-guid-3",
		Title:     "Test Title",
		Seeders:   ptrInt(5),
		MagnetURI: "magnet:?xt=urn:btih:abcd",
	}

	// Invoke the router's handler.
	err := router.handleIndexerResult(context.Background(), &IndexerResultEvent{Result: result})
	if err != nil {
		t.Fatalf("handleIndexerResult: %v", err)
	}

	// Check that DownloadFailed was emitted (no clients available).
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	ev := bus.events[0].data.(*DownloadFailureEvent)
	if ev.Topic() != TopicDownloadFailed {
		t.Errorf("Topic = %q, want %q", ev.Topic(), TopicDownloadFailed)
	}
}

func TestRouterFiltersLowSeedTorrents(t *testing.T) {
	t.Parallel()

	bus := &capturingBus{}
	router, _ := newTestRouter(t, bus)
	defer router.Close()

	// Register a test client.
	client := &fakeClient{id: "test-client"}
	if err := router.svc.registry.Register(client); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Publish a result with 0 seeders (should be filtered).
	result := &indexers.Result{
		GUID:      "test-guid-4",
		Title:     "No Seeders",
		Seeders:   ptrInt(0),
		MagnetURI: "magnet:?xt=urn:btih:abcd",
	}

	// Invoke the router's handler.
	err := router.handleIndexerResult(context.Background(), &IndexerResultEvent{Result: result})
	if err != nil {
		t.Fatalf("handleIndexerResult: %v", err)
	}

	// No events should be emitted (filtered out).
	if len(bus.events) != 0 {
		t.Fatalf("expected 0 events (filtered), got %d", len(bus.events))
	}
}

func TestRouterAcceptsUsenetResults(t *testing.T) {
	t.Parallel()

	bus := &capturingBus{}
	router, _ := newTestRouter(t, bus)
	defer router.Close()

	// Register a test client.
	client := &fakeClient{id: "test-client"}
	if err := router.svc.registry.Register(client); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Publish a usenet result (Seeders == nil).
	result := &indexers.Result{
		GUID:    "test-guid-5",
		Title:   "Usenet Release",
		Seeders: nil, // Usenet has no seeders
		Link:    "http://example.com/release.nzb",
	}

	// Invoke the router's handler.
	err := router.handleIndexerResult(context.Background(), &IndexerResultEvent{Result: result})
	if err != nil {
		t.Fatalf("handleIndexerResult: %v", err)
	}

	// DownloadQueued should be emitted (usenet passes filter).
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	ev := bus.events[0].data.(*DownloadQueuedEvent)
	if ev.Topic() != TopicDownloadQueued {
		t.Errorf("Topic = %q, want %q", ev.Topic(), TopicDownloadQueued)
	}
}

func TestBuildAddRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		result        *indexers.Result
		wantMagnet    string
		wantTorrentURL string
	}{
		{
			name: "prefer magnet",
			result: &indexers.Result{
				MagnetURI: "magnet:?xt=urn:btih:abc123",
				Link:      "http://example.com/torrent.torrent",
			},
			wantMagnet: "magnet:?xt=urn:btih:abc123",
		},
		{
			name: "fallback to infohash",
			result: &indexers.Result{
				Infohash: "abc123def456",
				Link:     "http://example.com/torrent.torrent",
			},
			wantMagnet: "magnet:?xt=urn:btih:abc123def456",
		},
		{
			name: "fallback to link",
			result: &indexers.Result{
				Link: "http://example.com/torrent.torrent",
			},
			wantTorrentURL: "http://example.com/torrent.torrent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := buildAddRequest(tt.result)
			if tt.wantMagnet != "" && req.Magnet != tt.wantMagnet {
				t.Errorf("Magnet = %q, want %q", req.Magnet, tt.wantMagnet)
			}
			if tt.wantTorrentURL != "" && req.TorrentURL != tt.wantTorrentURL {
				t.Errorf("TorrentURL = %q, want %q", req.TorrentURL, tt.wantTorrentURL)
			}
		})
	}
}

// ptrInt returns a pointer to an int.
func ptrInt(v int) *int { return &v }

// fakeClient is a test implementation of DownloadClient.
type fakeClient struct {
	id     string
	addErr error
}

func (c *fakeClient) ID() string           { return c.id }
func (c *fakeClient) Name() string         { return "Fake Client" }
func (c *fakeClient) Kind() Kind           { return KindNull }
func (c *fakeClient) Protocol() Protocol   { return ProtocolTorrent }
func (c *fakeClient) Add(ctx context.Context, req AddRequest) (AddResult, error) {
	if c.addErr != nil {
		return AddResult{}, c.addErr
	}
	return AddResult{ClientID: c.id, ItemID: "fake-item-123"}, nil
}
func (c *fakeClient) Status(ctx context.Context, ids ...string) ([]Item, error) {
	return nil, nil
}
func (c *fakeClient) Pause(ctx context.Context, ids ...string) error { return nil }
func (c *fakeClient) Resume(ctx context.Context, ids ...string) error { return nil }
func (c *fakeClient) Remove(ctx context.Context, ids []string, deleteFiles bool) error {
	return nil
}
func (c *fakeClient) SetPriority(_ context.Context, _ Priority, _ ...string) error   { return nil }
func (c *fakeClient) SetSpeedLimit(_ context.Context, _ int64, _ ...string) error    { return nil }
func (c *fakeClient) ForceStart(_ context.Context, _ ...string) error                { return nil }
func (c *fakeClient) Recheck(_ context.Context, _ ...string) error                   { return nil }
func (c *fakeClient) Reannounce(_ context.Context, _ ...string) error                { return nil }
func (c *fakeClient) Categories(ctx context.Context) ([]Category, error) { return nil, nil }
func (c *fakeClient) FreeSpace(ctx context.Context) (int64, error)       { return 0, nil }
func (c *fakeClient) Test(ctx context.Context) error                    { return nil }

// testRepository is a minimal Repository for testing.
type testRepository struct{}

func (r *testRepository) Create(ctx context.Context, def Definition) (Definition, error) {
	return Definition{}, nil
}
func (r *testRepository) Get(ctx context.Context, id string) (Definition, error) {
	return Definition{}, nil
}
func (r *testRepository) List(ctx context.Context) ([]Definition, error) {
	return []Definition{}, nil
}
func (r *testRepository) ListEnabled(ctx context.Context) ([]Definition, error) {
	return []Definition{}, nil
}
func (r *testRepository) Replace(ctx context.Context, def Definition) (Definition, error) {
	return Definition{}, nil
}
func (r *testRepository) Patch(ctx context.Context, p Patch) (Definition, error) {
	return Definition{}, nil
}
func (r *testRepository) Delete(ctx context.Context, id string) error {
	return nil
}
func (r *testRepository) UpsertHealth(ctx context.Context, h Health) error {
	return nil
}
func (r *testRepository) GetHealth(ctx context.Context, id string) (Health, error) {
	return Health{}, nil
}
func (r *testRepository) ListHealth(ctx context.Context) (map[string]Health, error) {
	return make(map[string]Health), nil
}
