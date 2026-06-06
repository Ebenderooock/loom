package plugins

import (
"context"
"database/sql"
"io"
"log/slog"
"net/http"
"net/http/httptest"
"path/filepath"
"sync"
"sync/atomic"
"testing"
"time"

"github.com/ebenderooock/loom/internal/kernel/config"
"github.com/ebenderooock/loom/internal/kernel/eventbus"
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

func quiet() *slog.Logger {
return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newRunner(t *testing.T, enabled bool) (*Runner, *Store) {
t.Helper()
store := NewStore(openTestDB(t))
r := NewRunner(eventbus.NewInProc(), store, func() bool { return enabled }, quiet())
return r, store
}

func TestStoreCRUDAndValidation(t *testing.T) {
store := NewStore(openTestDB(t))
ctx := context.Background()

// Missing source rejected.
if err := store.Create(ctx, &Plugin{Name: "x", Events: []string{"grab"}}); err == nil {
t.Fatal("expected error for empty source")
}
// Unknown event rejected.
if err := store.Create(ctx, &Plugin{Name: "x", Source: "1;", Events: []string{"nope"}}); err == nil {
t.Fatal("expected error for unknown event")
}

p := &Plugin{Name: "good", Source: `console.log("ok");`,
Events: []string{"grab", "import_complete"}, TimeoutSecs: 9999}
if err := store.Create(ctx, p); err != nil {
t.Fatalf("create: %v", err)
}
if p.ID == "" {
t.Fatal("expected ID assigned")
}
if p.TimeoutSecs != maxTimeoutSecs {
t.Fatalf("expected timeout capped at %d, got %d", maxTimeoutSecs, p.TimeoutSecs)
}

got, err := store.Get(ctx, p.ID)
if err != nil || got.Name != "good" || len(got.Events) != 2 {
t.Fatalf("get round-trip failed: %+v err=%v", got, err)
}

got.Enabled = true
if err := store.Update(ctx, got); err != nil {
t.Fatalf("update: %v", err)
}
subs, err := store.enabledForTopic(ctx, "grab")
if err != nil || len(subs) != 1 {
t.Fatalf("expected 1 enabled plugin for grab, got %d err=%v", len(subs), err)
}

if err := store.Delete(ctx, p.ID); err != nil {
t.Fatalf("delete: %v", err)
}
if _, err := store.Get(ctx, p.ID); err == nil {
t.Fatal("expected not-found after delete")
}
}

func TestRunOnceSuccessAndPayload(t *testing.T) {
r, store := newRunner(t, true)
ctx := context.Background()
p := &Plugin{Name: "echo", Enabled: true,
Source: `console.log("event:", event.event); console.log("title:", event.title);`,
Events: []string{"grab"}, TimeoutSecs: 10}
if err := store.Create(ctx, p); err != nil {
t.Fatalf("create: %v", err)
}

run := r.RunOnce(ctx, p)
if !run.Success || run.ExitCode != 0 {
t.Fatalf("expected success, got %+v", run)
}
if !contains(run.Stdout, "event: grab") || !contains(run.Stdout, "Test event from Loom") {
t.Fatalf("payload not visible to script: %q", run.Stdout)
}

runs, err := store.ListRuns(ctx, p.ID, 10)
if err != nil || len(runs) != 1 {
t.Fatalf("expected 1 recorded run, got %d err=%v", len(runs), err)
}
}

func TestOutputCapped(t *testing.T) {
r, store := newRunner(t, true)
ctx := context.Background()
p := &Plugin{Name: "spew", Enabled: true,
Source: `for (var i = 0; i < 200000; i++) { console.log("xxxxxxxxxxxxxxxxxxxx"); }`,
Events: []string{"grab"}, TimeoutSecs: 30}
if err := store.Create(ctx, p); err != nil {
t.Fatalf("create: %v", err)
}
run := r.RunOnce(ctx, p)
if !run.Success {
t.Fatalf("expected success, got %+v", run)
}
if len(run.Stdout) > maxOutputBytes+64 {
t.Fatalf("stdout not capped: %d bytes", len(run.Stdout))
}
if !contains(run.Stdout, "truncated") {
t.Fatalf("expected truncation marker, got %d bytes", len(run.Stdout))
}
}

// fakeEvent implements eventbus.Event plus the optional data interfaces.
type fakeEvent struct {
topic string
title string
data  map[string]any
}

func (e fakeEvent) Topic() string                    { return e.topic }
func (e fakeEvent) GetTitle() string                 { return e.title }
func (e fakeEvent) NotificationData() map[string]any { return e.data }

func TestEventDrivenExecution(t *testing.T) {
// A local server records that the plugin ran and received the payload.
var hits atomic.Int64
var gotTitle atomic.Value
gotTitle.Store("")
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
hits.Add(1)
body, _ := io.ReadAll(r.Body)
gotTitle.Store(string(body))
w.WriteHeader(http.StatusOK)
}))
defer srv.Close()

bus := eventbus.NewInProc()
store := NewStore(openTestDB(t))
r := NewRunner(bus, store, func() bool { return true }, quiet())
ctx := context.Background()

p := &Plugin{Name: "webhook", Enabled: true,
Source: `fetch("` + srv.URL + `", { method: "POST", body: event.title });`,
Events: []string{"grab"}, TimeoutSecs: 10}
if err := store.Create(ctx, p); err != nil {
t.Fatalf("create: %v", err)
}

r.Start(ctx)
defer r.Stop()

if err := bus.Publish(ctx, fakeEvent{topic: topicDownloadQueued, title: "Big Movie",
data: map[string]any{"title": "Big Movie"}}); err != nil {
t.Fatalf("publish: %v", err)
}

deadline := time.Now().Add(5 * time.Second)
for time.Now().Before(deadline) {
if hits.Load() > 0 && gotTitle.Load() == "Big Movie" {
return // plugin ran and received the payload
}
time.Sleep(50 * time.Millisecond)
}
t.Fatalf("plugin did not run on event within timeout (hits=%d title=%q)", hits.Load(), gotTitle.Load())
}

func TestDisabledFlagSkipsExecution(t *testing.T) {
bus := eventbus.NewInProc()
store := NewStore(openTestDB(t))
r := NewRunner(bus, store, func() bool { return false }, quiet())
ctx := context.Background()
p := &Plugin{Name: "noop", Enabled: true, Source: `1;`,
Events: []string{"grab"}, TimeoutSecs: 10}
if err := store.Create(ctx, p); err != nil {
t.Fatalf("create: %v", err)
}
r.Start(ctx)
defer r.Stop()
_ = bus.Publish(ctx, fakeEvent{topic: topicDownloadQueued, data: map[string]any{}})

time.Sleep(300 * time.Millisecond)
runs, _ := store.ListRuns(ctx, p.ID, 10)
if len(runs) != 0 {
t.Fatalf("expected no runs when feature disabled, got %d", len(runs))
}
}

// TestStopIsIdempotentAndSafeUnderLoad publishes events while stopping to prove
// the producer/consumer close coordination doesn't panic, and that Stop() can
// be called more than once.
func TestStopIsIdempotentAndSafeUnderLoad(t *testing.T) {
bus := eventbus.NewInProc()
store := NewStore(openTestDB(t))
r := NewRunner(bus, store, func() bool { return true }, quiet())
ctx := context.Background()
p := &Plugin{Name: "quick", Enabled: true, Source: `1;`,
Events: []string{"grab"}, TimeoutSecs: 10}
if err := store.Create(ctx, p); err != nil {
t.Fatalf("create: %v", err)
}
r.Start(ctx)

stop := make(chan struct{})
var pub sync.WaitGroup
pub.Add(1)
go func() {
defer pub.Done()
for {
select {
case <-stop:
return
default:
_ = bus.Publish(ctx, fakeEvent{topic: topicDownloadQueued, data: map[string]any{}})
}
}
}()

time.Sleep(50 * time.Millisecond)
r.Stop() // must not panic despite concurrent publishes
r.Stop() // idempotent
close(stop)
pub.Wait()
}

func TestUnknownTopicIgnored(t *testing.T) {
r, store := newRunner(t, true)
ctx := context.Background()
p := &Plugin{Name: "p", Enabled: true, Source: `1;`,
Events: []string{"grab"}, TimeoutSecs: 10}
if err := store.Create(ctx, p); err != nil {
t.Fatalf("create: %v", err)
}
r.Start(ctx)
defer r.Stop()
// A topic with no SupportedEvents mapping must be a no-op.
r.onEvent(fakeEvent{topic: "movies.added", data: map[string]any{}})
time.Sleep(150 * time.Millisecond)
runs, _ := store.ListRuns(ctx, p.ID, 10)
if len(runs) != 0 {
t.Fatalf("expected no runs for unmapped topic, got %d", len(runs))
}
}

func contains(haystack, needle string) bool {
return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
for i := 0; i+len(sub) <= len(s); i++ {
if s[i:i+len(sub)] == sub {
return i
}
}
return -1
}
