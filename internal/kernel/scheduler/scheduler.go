package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// HandlerFunc is the signature every registered job implements. The
// context passed in is canceled on scheduler shutdown; honour it.
type HandlerFunc func(ctx context.Context) error

// Status values recorded in scheduled_jobs.last_status.
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// Store is the persistence seam the scheduler depends on. It is
// satisfied by adapters in the storage package wrapping the
// sqlc-generated query packages.
type Store interface {
	// UpsertJob persists the row on first registration and refreshes
	// only the schedule + payload + next_run_at on subsequent calls;
	// run-status fields are preserved across restarts.
	UpsertJob(ctx context.Context, name, schedule string, payload []byte, nextRun time.Time) error

	// RecordRun stores the outcome of a single execution.
	RecordRun(ctx context.Context, name string, ranAt, nextRun time.Time, status, errMsg string) error

	// SetNextRun updates only the next_run_at column (used when a job
	// is skipped because the previous instance is still running).
	SetNextRun(ctx context.Context, name string, nextRun time.Time) error
}

// Clock abstracts time for tests. Production callers use SystemClock.
type Clock interface {
	Now() time.Time
}

// SystemClock is the real wall-clock implementation of Clock.
type SystemClock struct{}

// Now returns the current local time.
func (SystemClock) Now() time.Time { return time.Now() }

// Config controls scheduler behaviour. Zero values are filled with
// production-safe defaults by New.
type Config struct {
	// Enabled gates the entire loop. When false, Start is a no-op so
	// the binary can be deployed for HTTP-only debug.
	Enabled bool

	// Location is the timezone cron expressions are interpreted in.
	// Defaults to time.Local.
	Location *time.Location

	// ShutdownGrace bounds how long Stop will wait for in-flight jobs
	// before abandoning them. Defaults to 30 seconds.
	ShutdownGrace time.Duration

	// WithSeconds, when true, accepts 6-field cron expressions
	// (sec min hour dom month dow). The default 5-field form is used
	// otherwise.
	WithSeconds bool
}

// Scheduler runs registered jobs on a cron loop, persisting outcomes
// through a Store. It is safe for concurrent use after construction;
// Register may be called before or after Start.
type Scheduler struct {
	cfg    Config
	store  Store
	logger *slog.Logger
	clock  Clock
	parser cron.Parser

	mu       sync.Mutex
	jobs     map[string]*job
	stopping bool
	stopped  bool

	// loopCancel cancels the dispatch goroutine; jobCtx is the parent
	// context handed to in-flight handlers and is canceled by Stop.
	loopCancel context.CancelFunc
	jobCtx     context.Context
	jobCancel  context.CancelFunc
	wg         sync.WaitGroup
}

// job is the in-memory record for one registered handler.
type job struct {
	name     string
	schedule string
	payload  []byte
	handler  HandlerFunc
	parsed   cron.Schedule
	mu       sync.Mutex // ensures one in-flight run at a time
}

// New builds a Scheduler. The returned value is ready for Register and
// Start. Pass a non-nil Store; logger may be nil (slog.Default is
// used).
func New(cfg Config, store Store, logger *slog.Logger, clock Clock) (*Scheduler, error) {
	if store == nil {
		return nil, errors.New("scheduler: store must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = SystemClock{}
	}
	if cfg.Location == nil {
		cfg.Location = time.Local
	}
	if cfg.ShutdownGrace <= 0 {
		cfg.ShutdownGrace = 30 * time.Second
	}

	parserOpts := cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor
	if cfg.WithSeconds {
		parserOpts = cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor
	}

	return &Scheduler{
		cfg:    cfg,
		store:  store,
		logger: logger.With("module", "scheduler"),
		clock:  clock,
		parser: cron.NewParser(parserOpts),
		jobs:   make(map[string]*job),
	}, nil
}

// Register adds (or refreshes) a recurring job. It is idempotent: the
// schedule is reparsed, the row is upserted, and on schedule changes
// the in-memory cron entry takes effect on the next tick.
//
// Register may be called before or after Start. Registering a name
// that's already in the map updates the schedule and handler — useful
// for live-reload scenarios — but a run already in flight runs to
// completion under the previous handler.
func (s *Scheduler) Register(ctx context.Context, name, schedule string, handler HandlerFunc, payload []byte) error {
	if name == "" {
		return errors.New("scheduler: job name must not be empty")
	}
	if handler == nil {
		return fmt.Errorf("scheduler: handler for %q must not be nil", name)
	}
	parsed, err := s.parser.Parse(schedule)
	if err != nil {
		return fmt.Errorf("scheduler: parse schedule for %q: %w", name, err)
	}
	if payload == nil {
		payload = []byte("{}")
	}

	next := parsed.Next(s.clock.Now().In(s.cfg.Location))
	if err := s.store.UpsertJob(ctx, name, schedule, payload, next); err != nil {
		return fmt.Errorf("scheduler: persist job %q: %w", name, err)
	}

	s.mu.Lock()
	s.jobs[name] = &job{
		name:     name,
		schedule: schedule,
		payload:  payload,
		handler:  handler,
		parsed:   parsed,
	}
	s.mu.Unlock()

	s.logger.Info("registered job", "job", name, "schedule", schedule, "next_run_at", next)
	return nil
}

// Start launches the dispatch loop. It is a no-op when Config.Enabled
// is false or when the scheduler has already been started or stopped.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.stopped || s.loopCancel != nil {
		s.mu.Unlock()
		return
	}
	if !s.cfg.Enabled {
		s.logger.Info("scheduler disabled by config; not starting")
		s.mu.Unlock()
		return
	}
	loopCtx, loopCancel := context.WithCancel(ctx)
	jobCtx, jobCancel := context.WithCancel(context.Background())
	s.loopCancel = loopCancel
	s.jobCtx = jobCtx
	s.jobCancel = jobCancel
	s.mu.Unlock()

	s.wg.Add(1)
	go s.dispatch(loopCtx)
	s.logger.Info("scheduler started", "timezone", s.cfg.Location.String(), "with_seconds", s.cfg.WithSeconds)
}

// Stop signals shutdown, blocks until in-flight jobs finish or
// Config.ShutdownGrace elapses, and then returns. Calling Stop more
// than once is safe.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.stopping || s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopping = true
	cancel := s.loopCancel
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.waitForJobs()

	s.mu.Lock()
	if s.jobCancel != nil {
		s.jobCancel()
	}
	s.stopped = true
	s.mu.Unlock()
}

// waitForJobs blocks until the dispatch goroutine and any in-flight
// jobs return, or until ShutdownGrace elapses. After the deadline the
// per-job context is canceled so handlers that respect cancellation
// can wrap up; goroutines that ignore cancellation are simply
// abandoned and the binary can still exit.
func (s *Scheduler) waitForJobs() {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(s.cfg.ShutdownGrace):
		s.logger.Warn("shutdown grace exceeded; abandoning in-flight jobs",
			"grace", s.cfg.ShutdownGrace)
		s.mu.Lock()
		if s.jobCancel != nil {
			s.jobCancel()
		}
		s.mu.Unlock()
		// Give the cancel a brief window to unwind; then return
		// regardless so the binary can exit.
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
}

// dispatch is the single owner of the cron clock. It wakes once per
// second (or on context cancel) and fires any jobs whose next_run_at
// has elapsed.
func (s *Scheduler) dispatch(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.tickInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDueJobs(s.clock.Now().In(s.cfg.Location))
		}
	}
}

// tickInterval is one second when seconds are enabled, ten seconds
// otherwise. The longer cadence keeps idle CPU low for typical
// minute-resolution schedules.
func (s *Scheduler) tickInterval() time.Duration {
	if s.cfg.WithSeconds {
		return time.Second
	}
	return 10 * time.Second
}

// runDueJobs fires every job whose next_run_at is <= now. It snapshots
// the jobs map under the lock, then iterates without holding it so
// long-running handlers can't block Register or other dispatchers.
func (s *Scheduler) runDueJobs(now time.Time) {
	s.mu.Lock()
	if s.stopping || s.stopped {
		s.mu.Unlock()
		return
	}
	due := make([]*job, 0, len(s.jobs))
	for _, j := range s.jobs {
		// Compute the most recent scheduled fire time at or before
		// now: a job is due iff Next(time before now) is <= now.
		next := j.parsed.Next(now.Add(-s.tickInterval() - time.Second))
		if !next.After(now) {
			due = append(due, j)
		}
	}
	jobCtx := s.jobCtx
	s.mu.Unlock()

	for _, j := range due {
		s.fire(jobCtx, j, now)
	}
}

// fire runs a single job in its own goroutine, gated by the per-job
// mutex. If the mutex is already held the run is recorded as skipped
// and the next_run_at is bumped forward.
func (s *Scheduler) fire(ctx context.Context, j *job, now time.Time) {
	if !j.mu.TryLock() {
		nextRun := j.parsed.Next(now)
		if err := s.store.SetNextRun(context.Background(), j.name, nextRun); err != nil {
			s.logger.Error("update next_run_at after skip", "job", j.name, "err", err)
		}
		s.logger.Warn("job skipped: previous run still in flight", "job", j.name, "next_run_at", nextRun)
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer j.mu.Unlock()
		s.executeOnce(ctx, j, now)
	}()
}

// executeOnce runs the handler, captures panics and errors, and
// records the outcome.
func (s *Scheduler) executeOnce(ctx context.Context, j *job, scheduledFor time.Time) {
	startedAt := s.clock.Now()
	status := StatusSuccess
	errMsg := ""

	defer func() {
		if r := recover(); r != nil {
			status = StatusFailed
			errMsg = fmt.Sprintf("panic: %v", r)
			s.logger.Error("job panicked", "job", j.name, "panic", r)
		}
		nextRun := j.parsed.Next(s.clock.Now().In(s.cfg.Location))
		recordCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.store.RecordRun(recordCtx, j.name, startedAt, nextRun, status, errMsg); err != nil {
			s.logger.Error("record job run", "job", j.name, "err", err)
		}
	}()

	s.logger.Debug("job firing", "job", j.name, "scheduled_for", scheduledFor)
	if err := j.handler(ctx); err != nil {
		status = StatusFailed
		errMsg = err.Error()
		s.logger.Error("job failed", "job", j.name, "err", err)
		return
	}
	s.logger.Debug("job succeeded", "job", j.name, "duration", s.clock.Now().Sub(startedAt))
}

// JobNames returns the names of currently registered jobs in
// non-deterministic order. Intended for diagnostics.
func (s *Scheduler) JobNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.jobs))
	for name := range s.jobs {
		out = append(out, name)
	}
	return out
}
