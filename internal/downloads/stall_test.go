package downloads

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
)

type removingClient struct {
	fakeClientWithStatus
	mu      sync.Mutex
	removed []string
}

func (c *removingClient) Remove(_ context.Context, ids []string, _ bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removed = append(c.removed, ids...)
	return nil
}

type recordingNotifier struct {
	mu     sync.Mutex
	failed []string
}

func (n *recordingNotifier) NotifyDownloadComplete(string, string, string, string, string, string) {}
func (n *recordingNotifier) NotifyDownloadProgress(string, string, float64, int64, int64, float64, string, string, string) {
}
func (n *recordingNotifier) NotifyDownloadFailed(clientID, downloadID, _ string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failed = append(n.failed, clientID+":"+downloadID)
}

type stallFixture struct {
	monitor   *Monitor
	clock     *testClock
	client    *removingClient
	notifier  *recordingNotifier
	blocklist *BlocklistStore
	online    bool
}

func newStallFixture(t *testing.T) *stallFixture {
	t.Helper()
	f := &stallFixture{online: true}
	bus := &eventbusStub{capturingBusForMonitor: capturingBusForMonitor{events: []eventbus.Event{}}}
	f.monitor = newTestMonitor(t, bus)
	f.clock = f.monitor.clock.(*testClock)
	f.client = &removingClient{fakeClientWithStatus: fakeClientWithStatus{id: "qb"}}
	reg := NewRegistry()
	if err := reg.Register(f.client); err != nil {
		t.Fatalf("register: %v", err)
	}
	hist := openHistoryTestStore(t)
	f.blocklist = NewBlocklistStore(hist.db)
	f.notifier = &recordingNotifier{}
	f.monitor.stallTimeout = 30 * time.Minute
	f.monitor.noStartTimeout = 60 * time.Minute
	f.monitor.checkForStalled = true
	f.monitor.orchNotifier = f.notifier
	f.monitor.connectivityCheck = func(context.Context) bool { return f.online }
	f.monitor.stallHandler = NewStallHandler(StallHandlerOptions{
		Registry:  reg,
		Blocklist: f.blocklist,
		Clock:     f.clock,
	})
	return f
}

// sweep advances the clock by step and runs stall detection, n times.
func (f *stallFixture) sweep(t *testing.T, items []Item, step time.Duration, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		f.monitor.detectStalled(context.Background(), items)
		f.clock.now = f.clock.now.Add(step)
	}
}

const testHash = "0123456789ABCDEF0123456789ABCDEF01234567"

func stalledItem() Item {
	return Item{
		ID: testHash, ClientID: "qb", Title: "Movie.2024.1080p.WEB-DL",
		Status: StatusItemDownloading, Progress: 0.4, DownloadedBytes: 400,
	}
}

func TestStallDetectionRemovesBlocklistsAndNotifies(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t)
	items := []Item{stalledItem()}

	f.sweep(t, items, time.Minute, 30)
	if len(f.notifier.failed) != 0 {
		t.Fatalf("stalled too early: %v", f.notifier.failed)
	}
	f.sweep(t, items, time.Minute, 2)

	if len(f.notifier.failed) != 1 || f.notifier.failed[0] != "qb:"+testHash {
		t.Fatalf("expected orchestrator notified once, got %v", f.notifier.failed)
	}
	if len(f.client.removed) != 1 || f.client.removed[0] != testHash {
		t.Fatalf("expected stalled item removed from its client, got %v", f.client.removed)
	}
	ctx := context.Background()
	for _, tc := range []struct{ title, hash string }{
		{"", testHash},                     // same torrent, any indexer
		{"movie.2024.1080p.web-dl", ""},    // same release title
		{"Movie.2024.1080p.WEB-DL", "abc"}, // title hash
	} {
		blocked, err := f.blocklist.IsReleaseBlocked(ctx, tc.title, tc.hash)
		if err != nil || !blocked {
			t.Fatalf("expected %q/%q blocklisted, got %v (err %v)", tc.title, tc.hash, blocked, err)
		}
	}
	if blocked, _ := f.blocklist.IsReleaseBlocked(ctx, "Other.Release.2024", ""); blocked {
		t.Fatalf("unrelated release must not be blocklisted")
	}
}

func TestStallDetectionIgnoresNetworkOutage(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t)
	f.online = false
	items := []Item{stalledItem()}

	// Hours without progress while offline must not count.
	f.sweep(t, items, time.Minute, 240)
	if len(f.notifier.failed) != 0 || len(f.client.removed) != 0 {
		t.Fatalf("download abandoned during outage: notified=%v removed=%v", f.notifier.failed, f.client.removed)
	}

	// Once back online the full stall timeout must elapse again.
	f.online = true
	f.sweep(t, items, time.Minute, 20)
	if len(f.notifier.failed) != 0 {
		t.Fatalf("stall time from outage was counted")
	}
	f.sweep(t, items, time.Minute, 15)
	if len(f.notifier.failed) != 1 {
		t.Fatalf("expected stall after reconnect, got %v", f.notifier.failed)
	}
}

func TestStallDetectionOtherTrafficProvesConnectivity(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t)
	f.online = false // probe failing, but another torrent is seeding
	seeding := Item{ID: "seed", ClientID: "qb", Status: StatusItemSeeding, Progress: 1, UploadRate: 1000}
	items := []Item{stalledItem(), seeding}

	f.sweep(t, items, time.Minute, 32)
	if len(f.notifier.failed) != 1 {
		t.Fatalf("expected stall while other traffic flows, got %v", f.notifier.failed)
	}
}

func TestStallDetectionCatchesNeverStartedDownloads(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t)
	item := stalledItem()
	item.Progress, item.DownloadedBytes = 0, 0
	items := []Item{item}

	f.sweep(t, items, time.Minute, 45)
	if len(f.notifier.failed) != 0 {
		t.Fatalf("never-started download abandoned before no-start timeout")
	}
	f.sweep(t, items, time.Minute, 20)
	if len(f.notifier.failed) != 1 {
		t.Fatalf("expected never-started download to be abandoned, got %v", f.notifier.failed)
	}
}

func TestStallDetectionProgressResetsTimer(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t)
	item := stalledItem()
	for i := 0; i < 90; i++ {
		if i%20 == 0 {
			item.DownloadedBytes++
		}
		f.sweep(t, []Item{item}, time.Minute, 1)
	}
	if len(f.notifier.failed) != 0 {
		t.Fatalf("progressing download abandoned: %v", f.notifier.failed)
	}
}

func TestStallDetectionKeepsTrackingForUnreachableClient(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t)
	items := []Item{stalledItem()}
	f.sweep(t, items, time.Minute, 20)

	f.monitor.detectStalledSweep(context.Background(), nil, map[string]bool{"qb": true})
	if _, ok := f.monitor.lastProgress["qb:"+testHash]; !ok {
		t.Fatalf("tracking pruned for item on unreachable client")
	}
	f.monitor.detectStalledSweep(context.Background(), nil, nil)
	if _, ok := f.monitor.lastProgress["qb:"+testHash]; ok {
		t.Fatalf("tracking kept for item removed from reachable client")
	}
}

func TestStallHandlerPauseDoesNotAbandon(t *testing.T) {
	t.Parallel()
	f := newStallFixture(t)
	f.monitor.stallHandler.action = StallActionPause
	f.sweep(t, []Item{stalledItem()}, time.Minute, 35)
	if len(f.notifier.failed) != 0 || len(f.client.removed) != 0 {
		t.Fatalf("pause action must not abandon the download")
	}
}
