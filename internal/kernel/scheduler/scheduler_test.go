package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// memStore is an in-memory Store used by the scheduler tests. It is
// concurrency-safe and records every call so assertions can poke at
// the audit trail.
type memStore struct {
	mu      sync.Mutex
	upserts map[string]upsertCall
	runs    []runCall
	nexts   []nextCall

	upsertErr error
}

type upsertCall struct {
	schedule string
	payload  string
	nextRun  time.Time
}

type runCall struct {
	name    string
	ranAt   time.Time
	nextRun time.Time
	status  string
	errMsg  string
}

type nextCall struct {
	name    string
	nextRun time.Time
}

func newMemStore() *memStore {
	return &memStore{upserts: make(map[string]upsertCall)}
}

func (m *memStore) UpsertJob(_ context.Context, name, schedule string, payload []byte, nextRun time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.upserts[name] = upsertCall{schedule: schedule, payload: string(payload), nextRun: nextRun}
	return nil
}

func (m *memStore) RecordRun(_ context.Context, name string, ranAt, nextRun time.Time, status, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs = append(m.runs, runCall{name: name, ranAt: ranAt, nextRun: nextRun, status: status, errMsg: errMsg})
	return nil
}

func (m *memStore) SetNextRun(_ context.Context, name string, nextRun time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nexts = append(m.nexts, nextCall{name: name, nextRun: nextRun})
	return nil
}

func (m *memStore) runsFor(name string) []runCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]runCall, 0)
	for _, r := range m.runs {
		if r.name == name {
			out = append(out, r)
		}
	}
	return out
}

// fixedClock is a deterministic Clock for tests that need a frozen
// "now". Production uses SystemClock.
type fixedClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// --- Tests ----------------------------------------------------------

func TestParseSchedule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		schedule    string
		withSeconds bool
		wantErr     bool
	}{
		{"every minute", "* * * * *", false, false},
		{"every 6 hours", "0 */6 * * *", false, false},
		{"@hourly macro", "@hourly", false, false},
		{"@every duration", "@every 30s", false, false},
		{"with seconds", "*/5 * * * * *", true, false},
		{"empty rejected", "", false, true},
		{"junk rejected", "not a cron", false, true},
		{"too few fields without seconds", "* * *", false, true},
		{"five fields rejected when seconds required", "* * * * *", true, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := New(Config{Enabled: true, WithSeconds: tc.withSeconds}, newMemStore(), nil, nil)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			err = s.Register(context.Background(), "j", tc.schedule, func(context.Context) error { return nil }, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Register err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestRegisterIdempotent(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	s, err := New(Config{Enabled: true}, store, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := s.Register(context.Background(), "j", "*/5 * * * *", func(context.Context) error { return nil }, nil); err != nil {
			t.Fatalf("Register %d: %v", i, err)
		}
	}
	if got := len(s.JobNames()); got != 1 {
		t.Errorf("want 1 job after 3 registrations, got %d", got)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if u, ok := store.upserts["j"]; !ok || u.schedule != "*/5 * * * *" {
		t.Errorf("store missing or wrong schedule: %+v", u)
	}
}

func TestRegisterUpdatesScheduleOnReRegister(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	s, err := New(Config{Enabled: true}, store, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Register(context.Background(), "j", "*/5 * * * *", func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := s.Register(context.Background(), "j", "*/10 * * * *", func(context.Context) error { return nil }, nil); err != nil {
		t.Fatalf("second Register: %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if u := store.upserts["j"]; u.schedule != "*/10 * * * *" {
		t.Errorf("schedule not refreshed; got %q", u.schedule)
	}
}

func TestRegisterRejectsBadInput(t *testing.T) {
	t.Parallel()
	s, err := New(Config{Enabled: true}, newMemStore(), nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Register(context.Background(), "", "* * * * *", func(context.Context) error { return nil }, nil); err == nil {
		t.Errorf("empty name accepted")
	}
	if err := s.Register(context.Background(), "j", "* * * * *", nil, nil); err == nil {
		t.Errorf("nil handler accepted")
	}
}

// TestRunRecordsSuccess uses an @every-1s schedule with seconds enabled
// and waits up to 3s for a single run to be recorded as success.
func TestRunRecordsSuccess(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	s, err := New(Config{Enabled: true, WithSeconds: true, ShutdownGrace: time.Second}, store, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var hits atomic.Int32
	if err := s.Register(context.Background(), "ok", "@every 1s", func(context.Context) error {
		hits.Add(1)
		return nil
	}, nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	if !waitFor(3*time.Second, func() bool { return hits.Load() >= 1 && len(store.runsFor("ok")) >= 1 }) {
		t.Fatalf("expected at least one run; hits=%d runs=%d", hits.Load(), len(store.runsFor("ok")))
	}
	cancel()
	s.Stop()

	got := store.runsFor("ok")
	if got[0].status != StatusSuccess {
		t.Errorf("first run status=%q, want %q", got[0].status, StatusSuccess)
	}
	if got[0].errMsg != "" {
		t.Errorf("success run carries err=%q", got[0].errMsg)
	}
}

func TestRunRecordsFailure(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	s, err := New(Config{Enabled: true, WithSeconds: true, ShutdownGrace: time.Second}, store, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Register(context.Background(), "boom", "@every 1s", func(context.Context) error {
		return errors.New("kaboom")
	}, nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	if !waitFor(3*time.Second, func() bool { return len(store.runsFor("boom")) >= 1 }) {
		t.Fatalf("expected a recorded run")
	}
	cancel()
	s.Stop()

	got := store.runsFor("boom")
	if got[0].status != StatusFailed {
		t.Errorf("status=%q want %q", got[0].status, StatusFailed)
	}
	if got[0].errMsg != "kaboom" {
		t.Errorf("errMsg=%q want %q", got[0].errMsg, "kaboom")
	}
}

func TestPanicRecordedAsFailure(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	s, err := New(Config{Enabled: true, WithSeconds: true, ShutdownGrace: time.Second}, store, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Register(context.Background(), "panicky", "@every 1s", func(context.Context) error {
		panic("nope")
	}, nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	if !waitFor(3*time.Second, func() bool { return len(store.runsFor("panicky")) >= 1 }) {
		t.Fatalf("expected a recorded run after panic")
	}
	cancel()
	s.Stop()

	got := store.runsFor("panicky")
	if got[0].status != StatusFailed {
		t.Errorf("status=%q want %q", got[0].status, StatusFailed)
	}
}

// TestPerJobMutex ensures that when a previous run is still in flight,
// the next tick records a skip rather than running concurrently.
func TestPerJobMutex(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	s, err := New(Config{Enabled: true, WithSeconds: true, ShutdownGrace: 5 * time.Second}, store, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32
	release := make(chan struct{})
	if err := s.Register(context.Background(), "slow", "@every 1s", func(ctx context.Context) error {
		n := concurrent.Add(1)
		defer concurrent.Add(-1)
		for {
			cur := maxConcurrent.Load()
			if n <= cur || maxConcurrent.CompareAndSwap(cur, n) {
				break
			}
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}, nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)

	// Wait long enough for at least two ticks to fire.
	if !waitFor(4*time.Second, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return len(store.nexts) >= 1
	}) {
		t.Fatalf("expected at least one skip recorded; got nexts=%d", len(store.nexts))
	}

	close(release)
	cancel()
	s.Stop()

	if got := maxConcurrent.Load(); got > 1 {
		t.Errorf("concurrent runs of same job: %d (want 1)", got)
	}
}

// TestShutdownGraceWaitsForJobs verifies in-flight handlers see the
// configured grace before the dispatch loop returns.
func TestShutdownGraceWaitsForJobs(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	grace := 800 * time.Millisecond
	s, err := New(Config{Enabled: true, WithSeconds: true, ShutdownGrace: grace}, store, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	finished := make(chan struct{})
	if err := s.Register(context.Background(), "long", "@every 1s", func(ctx context.Context) error {
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
		}
		close(finished)
		return nil
	}, nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)

	// Wait for the handler to have actually started before stopping.
	if !waitFor(3*time.Second, func() bool { return len(store.runsFor("long")) == 0 && hasStarted(finished) }) {
		// fall through; we'll detect via the finished channel below
	}
	// Now request shutdown and confirm the handler completes before
	// Stop returns.
	cancel()
	stopReturned := make(chan struct{})
	go func() {
		s.Stop()
		close(stopReturned)
	}()

	select {
	case <-stopReturned:
	case <-time.After(grace + 2*time.Second):
		t.Fatal("Stop() did not return within grace + 2s")
	}
	select {
	case <-finished:
	default:
		t.Fatal("handler did not finish before Stop returned")
	}
}

// TestShutdownAbandonsUnresponsiveJobs verifies that when a handler
// ignores cancellation, Stop still returns within ShutdownGrace + a
// small slack.
func TestShutdownAbandonsUnresponsiveJobs(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	grace := 200 * time.Millisecond
	s, err := New(Config{Enabled: true, WithSeconds: true, ShutdownGrace: grace}, store, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	started := make(chan struct{}, 1)
	if err := s.Register(context.Background(), "stuck", "@every 1s", func(ctx context.Context) error {
		select {
		case started <- struct{}{}:
		default:
		}
		// Sleep ignoring cancel — simulates a misbehaving handler.
		time.Sleep(5 * time.Second)
		return nil
	}, nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("handler never started")
	}
	cancel()
	stopReturned := make(chan struct{})
	go func() { s.Stop(); close(stopReturned) }()
	select {
	case <-stopReturned:
	case <-time.After(grace + 3*time.Second):
		t.Fatalf("Stop blocked beyond grace (%s)", grace)
	}
	_ = store
}

func TestStartDisabledIsNoop(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	s, err := New(Config{Enabled: false, WithSeconds: true}, store, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var hits atomic.Int32
	if err := s.Register(context.Background(), "j", "@every 1s", func(context.Context) error {
		hits.Add(1)
		return nil
	}, nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	time.Sleep(1500 * time.Millisecond)
	s.Stop()
	if hits.Load() != 0 {
		t.Errorf("disabled scheduler ran %d jobs", hits.Load())
	}
}

func TestUpsertErrorPropagates(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	store.upsertErr = fmt.Errorf("disk full")
	s, err := New(Config{Enabled: true}, store, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Register(context.Background(), "j", "* * * * *", func(context.Context) error { return nil }, nil); err == nil {
		t.Errorf("expected upsert error to surface")
	}
}

func TestNewRequiresStore(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{Enabled: true}, nil, nil, nil); err == nil {
		t.Errorf("expected error for nil store")
	}
}

func TestRegisterHousekeepingValidatesEngine(t *testing.T) {
	t.Parallel()
	s, err := New(Config{Enabled: true}, newMemStore(), nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := RegisterHousekeeping(context.Background(), s, nil, EngineSQLite); err == nil {
		t.Errorf("expected error for nil db")
	}
	if err := RegisterHousekeeping(context.Background(), nil, nil, EngineSQLite); err == nil {
		t.Errorf("expected error for nil scheduler")
	}
}

// --- helpers --------------------------------------------------------

func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

func hasStarted(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
