// Package scheduler runs cron-like and one-shot tasks. The Phase-1 impl
// is in-memory; persistence to the storage layer lands alongside the DB.
package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Task is a unit of recurring work.
type Task struct {
	Name     string
	Interval time.Duration
	Jitter   time.Duration // +/- jitter applied each run
	Run      func(ctx context.Context) error
}

// Scheduler runs registered Tasks on a clock. It is safe for concurrent use.
type Scheduler struct {
	clock   Clock
	tasks   []Task
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	started atomic.Bool
}

// Clock is the time source. The default is time.Now / time.NewTimer.
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) *time.Timer
}

type realClock struct{}

func (realClock) Now() time.Time                  { return time.Now() }
func (realClock) NewTimer(d time.Duration) *time.Timer { return time.NewTimer(d) }

// New returns a Scheduler using the real clock.
func New() *Scheduler { return &Scheduler{clock: realClock{}} }

// WithClock sets a custom Clock; intended for tests.
func (s *Scheduler) WithClock(c Clock) *Scheduler { s.clock = c; return s }

// Register adds a task. Must be called before Start.
func (s *Scheduler) Register(t Task) {
	if s.started.Load() {
		panic("scheduler: Register after Start")
	}
	s.tasks = append(s.tasks, t)
}

// Start launches a goroutine per task. Stop cancels all of them.
func (s *Scheduler) Start(ctx context.Context) {
	if !s.started.CompareAndSwap(false, true) {
		return
	}
	ctx, s.cancel = context.WithCancel(ctx)
	for _, t := range s.tasks {
		t := t
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.loop(ctx, t)
		}()
	}
}

// Stop cancels and waits for tasks to finish.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *Scheduler) loop(ctx context.Context, t Task) {
	for {
		d := t.Interval + jitter(t.Jitter)
		if d <= 0 {
			d = time.Second
		}
		timer := s.clock.NewTimer(d)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if err := t.Run(ctx); err != nil {
			// Tasks own their error reporting; the scheduler does not
			// log here to avoid coupling to the logger package.
			_ = err
		}
	}
}

// jitter returns a uniform value in [-d, d). Replaced in tests via Clock if
// determinism is required.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return time.Duration(time.Now().UnixNano() % int64(d)) // cheap; PRNG TODO
}
