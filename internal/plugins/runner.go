package plugins

import (
"context"
"log/slog"
"sync"
"time"

"github.com/ebenderooock/loom/internal/kernel/eventbus"
)

const (
workerCount    = 4
queueSize      = 128
maxOutputBytes = 64 * 1024 // per stream (stdout/stderr) cap
drainTimeout   = 30 * time.Second
cancelWait     = 5 * time.Second
)

// eventJob is one delivered event; a worker expands it to the matching plugins.
type eventJob struct {
def     EventDef
payload Payload
}

// Runner subscribes to the event bus and executes matching JavaScript plugins in
// a bounded worker pool, fully decoupled from the bus's own goroutines.
type Runner struct {
bus     eventbus.Bus
store   *Store
enabled func() bool // feature-flag gate
logger  *slog.Logger

jobs   chan eventJob
unsubs []func()
wg     sync.WaitGroup // worker pool

rootCtx context.Context
cancel  context.CancelFunc

// Producer coordination so we can close(jobs) without racing onEvent senders.
prodMu   sync.Mutex
stopping bool
prodWG   sync.WaitGroup
stopOnce sync.Once
}

// NewRunner builds a runner. enabled gates execution (feature flag); a nil
// enabled func is treated as always-enabled.
func NewRunner(bus eventbus.Bus, store *Store, enabled func() bool, logger *slog.Logger) *Runner {
if enabled == nil {
enabled = func() bool { return true }
}
return &Runner{
bus:     bus,
store:   store,
enabled: enabled,
logger:  logger.With("component", "plugin-runner"),
jobs:    make(chan eventJob, queueSize),
}
}

// Start subscribes to all supported topics and launches the worker pool.
func (r *Runner) Start(ctx context.Context) {
r.rootCtx, r.cancel = context.WithCancel(ctx)
for i := 0; i < workerCount; i++ {
r.wg.Add(1)
go r.worker()
}
for _, e := range SupportedEvents {
unsub := r.bus.Subscribe(e.Topic, func(_ context.Context, ev eventbus.Event) error {
r.onEvent(ev)
return nil
})
r.unsubs = append(r.unsubs, unsub)
}
r.logger.Info("plugin runner started", "topics", len(SupportedEvents), "workers", workerCount)
}

// Stop unsubscribes, drains in-flight work within a timeout, then interrupts any
// scripts still running. Idempotent.
func (r *Runner) Stop() {
r.stopOnce.Do(r.stop)
}

func (r *Runner) stop() {
// 1. Stop receiving new events.
for _, u := range r.unsubs {
u()
}
r.unsubs = nil

// 2. Block new producers, wait for in-flight onEvent sends, then close.
r.prodMu.Lock()
r.stopping = true
r.prodMu.Unlock()
r.prodWG.Wait()
close(r.jobs)

// 3. Wait for workers to drain, up to the drain timeout.
done := make(chan struct{})
go func() {
r.wg.Wait()
close(done)
}()
select {
case <-done:
r.logger.Info("plugin runner stopped")
return
case <-time.After(drainTimeout):
r.logger.Warn("plugin runner drain timed out; interrupting in-flight scripts")
}

// 4. Cancel the root context: executeJS watches it and interrupts the VM, so
// any in-flight script unwinds. We never block forever.
if r.cancel != nil {
r.cancel()
}
select {
case <-done:
case <-time.After(cancelWait):
r.logger.Error("plugin runner did not stop cleanly; abandoning workers")
}
r.logger.Info("plugin runner stopped")
}

// onEvent runs on a bus goroutine: it must be cheap. It builds the payload and
// enqueues a single job (no DB access here); a worker expands it to plugins.
func (r *Runner) onEvent(ev eventbus.Event) {
if !r.enabled() {
return
}
def, ok := eventByTopic(ev.Topic())
if !ok {
return
}

// Register as a producer so Stop() won't close(jobs) mid-send.
r.prodMu.Lock()
if r.stopping {
r.prodMu.Unlock()
return
}
r.prodWG.Add(1)
r.prodMu.Unlock()
defer r.prodWG.Done()

job := eventJob{def: def, payload: buildPayload(def, ev)}
select {
case r.jobs <- job:
default:
r.logger.Warn("plugin queue full, dropping event", "event", def.Key)
}
}

func (r *Runner) worker() {
defer r.wg.Done()
for j := range r.jobs {
r.dispatch(j)
}
}

// dispatch finds the plugins subscribed to an event and runs each (off the bus).
func (r *Runner) dispatch(j eventJob) {
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
plugins, err := r.store.enabledForTopic(ctx, j.def.Key)
cancel()
if err != nil {
r.logger.Error("plugin lookup failed", "event", j.def.Key, "error", err)
return
}
for _, p := range plugins {
r.execute(r.rootCtx, p, j.payload)
}
}

// execute runs one plugin, recovering host panics so a misbehaving script can
// never take down a worker. parent bounds the run's lifetime (cancelled on
// shutdown or client disconnect).
func (r *Runner) execute(parent context.Context, p *Plugin, payload Payload) (result *Run) {
defer func() {
if rec := recover(); rec != nil {
r.logger.Error("plugin execution panicked", "plugin", p.Name, "panic", rec)
result = &Run{PluginID: p.ID, PluginName: p.Name, Topic: payload.Topic,
Success: false, ExitCode: -1, ErrorMsg: "panicked during execution", StartedAt: time.Now().UTC()}
r.persistRun(result)
}
}()
if parent == nil {
parent = context.Background()
}
return r.executeJS(parent, p, payload)
}

func (r *Runner) persistRun(run *Run) {
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := r.store.InsertRun(ctx, run); err != nil {
r.logger.Warn("failed to record plugin run", "plugin", run.PluginName, "error", err)
}
}

func (r *Runner) recordSkip(p *Plugin, topic, reason string) *Run {
run := &Run{
PluginID: p.ID, PluginName: p.Name, Topic: topic,
Success: false, ExitCode: -1, ErrorMsg: "skipped: " + reason, StartedAt: time.Now().UTC(),
}
r.persistRun(run)
return run
}

// RunOnce executes a plugin synchronously with a synthetic payload (used by the
// "test" endpoint) and returns the recorded run. The request context bounds the
// execution, so a client disconnect cancels it.
func (r *Runner) RunOnce(ctx context.Context, p *Plugin) *Run {
def := EventDef{Key: "test", Topic: "test", Label: "Test"}
if len(p.Events) > 0 {
if d, ok := eventByKey(p.Events[0]); ok {
def = d
}
}
payload := Payload{
Version: PayloadVersion, Event: def.Key, Topic: def.Topic,
Title: "Test event from Loom", Timestamp: time.Now().UTC(),
Data: map[string]any{"title": "Test event from Loom", "test": true},
}
return r.execute(ctx, p, payload)
}
