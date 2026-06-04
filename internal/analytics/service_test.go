package analytics

import (
	"context"
	"database/sql"
	"io"
	"sync"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/connect"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(dir, "loom.db")},
	}
	db, err := storage.Open(context.Background(), cfg,
		slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db.DB()
}

type fakeSource struct{ conns []*connect.Connection }

func (f *fakeSource) ListConnections(_ context.Context) ([]*connect.Connection, error) {
	return f.conns, nil
}

func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// captureBus records published events for assertions. It satisfies
// eventbus.Bus; Subscribe is unused in these tests.
type captureBus struct {
	mu     sync.Mutex
	events []eventbus.Event
}

func (b *captureBus) Publish(_ context.Context, ev eventbus.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
	return nil
}

func (b *captureBus) Subscribe(string, eventbus.Handler) func() { return func() {} }

func (b *captureBus) topics() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.events))
	for i, e := range b.events {
		out[i] = e.Topic()
	}
	return out
}

func newSvc(t *testing.T, conns ...*connect.Connection) (*Service, *Store, *captureBus) {
	t.Helper()
	db := openTestDB(t)
	store := NewStore(db)
	bus := &captureBus{}
	svc := NewService(store, &fakeSource{conns: conns}, bus, quiet())
	return svc, store, bus
}

func plexConn(id, name string) *connect.Connection {
	return &connect.Connection{ID: id, Name: name, Provider: connect.ProviderPlex, Enabled: true}
}

func TestPersistContinuityAccumulatesWatched(t *testing.T) {
	svc, store, _ := newSvc(t)
	conn := plexConn("c1", "Plex")
	ctx := context.Background()
	grace := 60 * time.Second

	sess := connect.Session{SessionKey: "10", MediaID: "100", User: "alice", MediaType: "episode",
		Title: "Ep", GrandparentTitle: "Show", FullTitle: "Show - S01E01 - Ep", State: "playing",
		PositionMs: 1000, DurationMs: 600000}

	t0 := time.Date(2026, 6, 4, 20, 0, 0, 0, time.UTC)
	svc.persistConnection(ctx, conn, []connect.Session{sess}, t0, grace, false)
	svc.persistConnection(ctx, conn, []connect.Session{sess}, t0.Add(30*time.Second), grace, false)

	open, err := store.OpenRowsForConn(ctx, "c1")
	if err != nil {
		t.Fatalf("open rows: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected 1 open row (continuous session), got %d", len(open))
	}
	if open[0].WatchedMs != 30000 {
		t.Fatalf("expected 30000ms watched, got %d", open[0].WatchedMs)
	}
}

func TestPersistCapsDeltaAtGrace(t *testing.T) {
	svc, store, _ := newSvc(t)
	conn := plexConn("c1", "Plex")
	ctx := context.Background()
	grace := 60 * time.Second
	sess := connect.Session{SessionKey: "10", MediaID: "100", State: "playing", DurationMs: 600000}

	t0 := time.Date(2026, 6, 4, 20, 0, 0, 0, time.UTC)
	svc.persistConnection(ctx, conn, []connect.Session{sess}, t0, grace, false)
	// A long gap (server unreachable): delta is 10m but must cap at grace (60s).
	svc.persistConnection(ctx, conn, []connect.Session{sess}, t0.Add(10*time.Minute), grace, false)

	open, _ := store.OpenRowsForConn(ctx, "c1")
	if len(open) != 1 || open[0].WatchedMs != 60000 {
		t.Fatalf("expected watched capped at 60000ms, got %+v", open)
	}
}

func TestPausedDoesNotAccumulate(t *testing.T) {
	svc, store, _ := newSvc(t)
	conn := plexConn("c1", "Plex")
	ctx := context.Background()
	grace := 60 * time.Second
	playing := connect.Session{SessionKey: "10", MediaID: "100", State: "playing", DurationMs: 600000}
	paused := playing
	paused.State = "paused"

	t0 := time.Date(2026, 6, 4, 20, 0, 0, 0, time.UTC)
	svc.persistConnection(ctx, conn, []connect.Session{playing}, t0, grace, false)
	svc.persistConnection(ctx, conn, []connect.Session{paused}, t0.Add(30*time.Second), grace, false)

	open, _ := store.OpenRowsForConn(ctx, "c1")
	if len(open) != 1 || open[0].WatchedMs != 0 {
		t.Fatalf("paused session should not accumulate watched time, got %+v", open)
	}
}

func TestReapOnDisappear(t *testing.T) {
	svc, store, _ := newSvc(t)
	conn := plexConn("c1", "Plex")
	ctx := context.Background()
	grace := 60 * time.Second
	sess := connect.Session{SessionKey: "10", MediaID: "100", State: "playing", DurationMs: 600000}

	t0 := time.Date(2026, 6, 4, 20, 0, 0, 0, time.UTC)
	svc.persistConnection(ctx, conn, []connect.Session{sess}, t0, grace, false)

	// Disappears; now is past lastSeen + grace, so it must be closed.
	svc.persistConnection(ctx, conn, nil, t0.Add(2*grace+time.Second), grace, false)

	open, _ := store.OpenRowsForConn(ctx, "c1")
	if len(open) != 0 {
		t.Fatalf("expected disappeared session to be reaped, %d still open", len(open))
	}
	hist, _ := store.ListHistory(ctx, HistoryFilter{})
	if len(hist) != 1 || hist[0].EndedAt == nil {
		t.Fatalf("expected 1 ended history row, got %+v", hist)
	}
}

func TestReapWaitsForGrace(t *testing.T) {
	svc, store, _ := newSvc(t)
	conn := plexConn("c1", "Plex")
	ctx := context.Background()
	grace := 60 * time.Second
	sess := connect.Session{SessionKey: "10", MediaID: "100", State: "playing", DurationMs: 600000}

	t0 := time.Date(2026, 6, 4, 20, 0, 0, 0, time.UTC)
	svc.persistConnection(ctx, conn, []connect.Session{sess}, t0, grace, false)
	// Missed a single poll (30s) — within grace, must NOT reap.
	svc.persistConnection(ctx, conn, nil, t0.Add(30*time.Second), grace, false)

	open, _ := store.OpenRowsForConn(ctx, "c1")
	if len(open) != 1 {
		t.Fatalf("a single missed poll should not reap the session, %d open", len(open))
	}
}

func TestDistinctMediaSameSessionKey(t *testing.T) {
	svc, store, _ := newSvc(t)
	conn := plexConn("c1", "Plex")
	ctx := context.Background()
	grace := 60 * time.Second
	t0 := time.Date(2026, 6, 4, 20, 0, 0, 0, time.UTC)

	a := connect.Session{SessionKey: "10", MediaID: "100", State: "playing", DurationMs: 1}
	b := connect.Session{SessionKey: "10", MediaID: "200", State: "playing", DurationMs: 1}
	svc.persistConnection(ctx, conn, []connect.Session{a, b}, t0, grace, false)

	open, _ := store.OpenRowsForConn(ctx, "c1")
	if len(open) != 2 {
		t.Fatalf("same session key but different media must be 2 rows, got %d", len(open))
	}
}

func TestResetOrphansClosesOpenRows(t *testing.T) {
	svc, store, _ := newSvc(t)
	conn := plexConn("c1", "Plex")
	ctx := context.Background()
	sess := connect.Session{SessionKey: "10", MediaID: "100", State: "playing", DurationMs: 1}
	svc.persistConnection(ctx, conn, []connect.Session{sess}, time.Now().UTC(), 60*time.Second, false)

	svc.ResetOrphans(ctx)

	open, _ := store.OpenRowsForConn(ctx, "c1")
	if len(open) != 0 {
		t.Fatalf("ResetOrphans should close all open rows, %d still open", len(open))
	}
}

func TestStatsThresholdAndAggregates(t *testing.T) {
	svc, store, _ := newSvc(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// One qualifying play (>=60s) and one below threshold.
	must := func(r HistoryRecord) {
		if err := store.InsertOpen(ctx, r); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	must(HistoryRecord{ID: "a", ConnectionID: "c1", Provider: "plex", SessionKey: "1", MediaID: "m1",
		User: "alice", MediaType: "movie", FullTitle: "Movie A", StartedAt: now, LastSeenAt: now, WatchedMs: 120000})
	must(HistoryRecord{ID: "b", ConnectionID: "c1", Provider: "plex", SessionKey: "2", MediaID: "m2",
		User: "bob", MediaType: "movie", FullTitle: "Movie B", StartedAt: now, LastSeenAt: now, WatchedMs: 5000})

	stats, err := svc.Stats(ctx, 30)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Totals.Plays != 1 {
		t.Fatalf("expected 1 play above threshold, got %d", stats.Totals.Plays)
	}
	if stats.Totals.UniqueUsers != 1 {
		t.Fatalf("expected 1 unique user, got %d", stats.Totals.UniqueUsers)
	}
	if len(stats.TopUsers) != 1 || stats.TopUsers[0].User != "alice" {
		t.Fatalf("expected alice as top user, got %+v", stats.TopUsers)
	}
	if len(stats.TopMedia) != 1 || stats.TopMedia[0].Title != "Movie A" {
		t.Fatalf("expected Movie A as top media, got %+v", stats.TopMedia)
	}
}

func TestSampleIsolatesFailedConnection(t *testing.T) {
	connA := plexConn("a", "A")
	connB := plexConn("b", "B")
	svc, store, _ := newSvc(t, connA, connB)
	ctx := context.Background()

	failB := false
	svc.fetch = func(_ context.Context, c *connect.Connection) ([]connect.Session, error) {
		if c.ID == "b" && failB {
			return nil, context.DeadlineExceeded
		}
		return []connect.Session{{SessionKey: "1", MediaID: c.ID + "-m", State: "playing", DurationMs: 100, FullTitle: c.Name}}, nil
	}

	svc.Sample(ctx, 30*time.Second)
	if len(svc.ActiveStreams()) != 2 {
		t.Fatalf("expected 2 live streams after first sample, got %d", len(svc.ActiveStreams()))
	}

	// Now B fails: its stream must be retained (not dropped) and its open row
	// must not be reaped.
	failB = true
	svc.Sample(ctx, 30*time.Second)
	if len(svc.ActiveStreams()) != 2 {
		t.Fatalf("failed connection's stream should be retained, got %d", len(svc.ActiveStreams()))
	}
	openB, _ := store.OpenRowsForConn(ctx, "b")
	if len(openB) != 1 {
		t.Fatalf("failed connection's history must not be reaped, %d open", len(openB))
	}
}

func countTopic(topics []string, want string) int {
n := 0
for _, tp := range topics {
if tp == want {
n++
}
}
return n
}

func topicsOf(evs []*PlaybackEvent) []string {
out := make([]string, 0, len(evs))
for _, ev := range evs {
out = append(out, ev.Topic())
}
return out
}

func TestSamplePublishesStartAndStop(t *testing.T) {
conn := plexConn("c1", "Plex")
svc, _, bus := newSvc(t, conn)
ctx := context.Background()

playing := false
svc.fetch = func(_ context.Context, c *connect.Connection) ([]connect.Session, error) {
if !playing {
return nil, nil
}
return []connect.Session{{SessionKey: "1", MediaID: "m1", User: "alice",
MediaType: "movie", FullTitle: "Movie A", State: "playing", DurationMs: 600000}}, nil
}

// Baseline sample (nothing playing) primes the service past the startup pass.
svc.Sample(ctx, 30*time.Second)
if got := len(bus.topics()); got != 0 {
t.Fatalf("baseline sample must emit nothing, got %v", bus.topics())
}

// Now a session appears -> one start event.
playing = true
svc.Sample(ctx, 30*time.Second)
if got := countTopic(bus.topics(), TopicPlaybackStarted); got != 1 {
t.Fatalf("expected 1 start event, got %d (%v)", got, bus.topics())
}

// Second sample with the same session must NOT emit another start.
svc.Sample(ctx, 30*time.Second)
if got := countTopic(bus.topics(), TopicPlaybackStarted); got != 1 {
t.Fatalf("expected still 1 start event after continuity, got %d", got)
}

// Stop playing; one tick later it's within grace -> no stop yet.
playing = false
svc.Sample(ctx, 30*time.Second)
if got := countTopic(bus.topics(), TopicPlaybackStopped); got != 0 {
t.Fatalf("one-tick disappearance must not emit a stop, got %d", got)
}
}

func TestFirstSampleSuppressesStartEvents(t *testing.T) {
conn := plexConn("c1", "Plex")
svc, _, bus := newSvc(t, conn)
ctx := context.Background()

svc.fetch = func(_ context.Context, c *connect.Connection) ([]connect.Session, error) {
return []connect.Session{{SessionKey: "1", MediaID: "m1", User: "alice",
	MediaType: "movie", FullTitle: "Movie A", State: "playing", DurationMs: 600000}}, nil
}

// The baseline sample after startup must not emit a start event for a stream
// that was already running before Loom started (avoids restart spam).
svc.Sample(ctx, 30*time.Second)
if got := countTopic(bus.topics(), TopicPlaybackStarted); got != 0 {
t.Fatalf("baseline sample must suppress start events, got %d (%v)", got, bus.topics())
}
}

func TestPersistConnectionReturnsStartAndStopEvents(t *testing.T) {
conn := plexConn("c1", "Plex")
svc, _, _ := newSvc(t, conn)
ctx := context.Background()
grace := 60 * time.Second
t0 := time.Date(2026, 6, 4, 20, 0, 0, 0, time.UTC)
sess := connect.Session{SessionKey: "1", MediaID: "m1", User: "alice", MediaType: "movie",
FullTitle: "Movie A", State: "playing", DurationMs: 600000}

// New row -> exactly one start event returned.
_, evs := svc.persistConnection(ctx, conn, []connect.Session{sess}, t0, grace, false)
if countTopic(topicsOf(evs), TopicPlaybackStarted) != 1 || len(evs) != 1 {
t.Fatalf("expected 1 start event, got %v", topicsOf(evs))
}

// Suppressed baseline -> no start event for the same continuing session.
_, evs = svc.persistConnection(ctx, conn, []connect.Session{sess}, t0.Add(30*time.Second), grace, true)
if len(evs) != 0 {
t.Fatalf("continuing session must not emit events, got %v", topicsOf(evs))
}

// Session gone past grace -> exactly one stop event returned.
_, evs = svc.persistConnection(ctx, conn, nil, t0.Add(2*grace+time.Second), grace, false)
if countTopic(topicsOf(evs), TopicPlaybackStopped) != 1 || len(evs) != 1 {
t.Fatalf("expected 1 stop event after grace, got %v", topicsOf(evs))
}
}

func TestStartupReapEmitsNoEvents(t *testing.T) {
conn := plexConn("c1", "Plex")
svc, _, bus := newSvc(t, conn)
ctx := context.Background()
sess := connect.Session{SessionKey: "1", MediaID: "m1", State: "playing", DurationMs: 600000}
svc.persistConnection(ctx, conn, []connect.Session{sess}, time.Now().UTC(), 60*time.Second, false)

svc.ResetOrphans(ctx)

if len(bus.topics()) != 0 {
t.Fatalf("startup reap must not publish events, got %v", bus.topics())
}
}

func TestStickyTranscodeCounts(t *testing.T) {
svc, _, _ := newSvc(t)
conn := plexConn("c1", "Plex")
ctx := context.Background()
grace := 60 * time.Second
t0 := time.Date(2026, 6, 4, 20, 0, 0, 0, time.UTC)

// First sample: transcoding. Second sample: direct-play. The play must still
// count as a transcode (sticky) and the bitrate must be retained.
transcoding := connect.Session{SessionKey: "1", MediaID: "m1", User: "alice", MediaType: "movie",
FullTitle: "Movie A", State: "playing", DurationMs: 600000, Transcode: true, BitrateKbps: 8000}
direct := transcoding
direct.Transcode = false
direct.BitrateKbps = 0

svc.persistConnection(ctx, conn, []connect.Session{transcoding}, t0, grace, false)
svc.persistConnection(ctx, conn, []connect.Session{direct}, t0.Add(90*time.Second), grace, false)

stats, err := svc.Stats(ctx, 30)
if err != nil {
t.Fatalf("stats: %v", err)
}
if stats.Totals.TranscodePlays != 1 || stats.Totals.DirectPlays != 0 {
t.Fatalf("expected sticky transcode (1 transcode, 0 direct), got %+v", stats.Totals)
}
if stats.Totals.AvgBitrateKbps != 8000 {
t.Fatalf("expected retained 8000 kbps avg, got %d", stats.Totals.AvgBitrateKbps)
}
}
