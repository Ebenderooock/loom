// Package eventbus is an in-process, typed publish/subscribe bus used by
// Loom modules to communicate without taking direct dependencies on each
// other. The interface is defined to match what an embedded NATS backend
// will provide in split-mode (Phase 11), so downstream code is unchanged.
package eventbus

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultMaxWorkers is the default concurrency limit for async dispatch.
const DefaultMaxWorkers = 32

// Event is anything that has a topic. Concrete events live in module
// packages (e.g. indexers.GrabRequested) so dependencies stay one-way.
type Event interface {
	Topic() string
}

// Handler is invoked once per published event of its subscribed topic.
type Handler func(ctx context.Context, ev Event) error

// Bus is the public interface; tests can replace it freely.
type Bus interface {
	Publish(ctx context.Context, ev Event) error
	Subscribe(topic string, h Handler) (unsubscribe func())
}

// AsyncBus extends Bus with ordered subscriptions and graceful shutdown.
type AsyncBus interface {
	Bus
	// SubscribeOrdered registers a handler that executes serially for a topic.
	SubscribeOrdered(topic string, h Handler) (unsubscribe func())
	// Close stops accepting publishes and waits for in-flight handlers.
	Close(timeout time.Duration) error
}

// Option configures an InProc bus.
type Option func(*inproc)

// WithMaxWorkers sets the maximum number of concurrent handler goroutines.
func WithMaxWorkers(n int) Option {
	return func(b *inproc) {
		if n > 0 {
			b.sem = make(chan struct{}, n)
		}
	}
}

type subscription struct {
	id        uint64
	h         Handler
	ordered   bool
	queue     chan dispatchItem // non-nil for ordered subscriptions
	closeOnce sync.Once        // ensures queue is closed exactly once
}

type dispatchItem struct {
	ctx context.Context
	ev  Event
}

type inproc struct {
	mu     sync.RWMutex
	subs   map[string][]*subscription
	nextID atomic.Uint64
	sem    chan struct{} // semaphore limiting concurrent handlers
	wg     sync.WaitGroup
	closed atomic.Bool
}

// NewInProc returns an async in-process bus. Safe for concurrent use.
func NewInProc(opts ...Option) AsyncBus {
	b := &inproc{
		subs: make(map[string][]*subscription),
		sem:  make(chan struct{}, DefaultMaxWorkers),
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

func (b *inproc) Publish(ctx context.Context, ev Event) error {
	if b.closed.Load() {
		return ErrBusClosed
	}
	b.mu.RLock()
	subs := append([]*subscription(nil), b.subs[ev.Topic()]...)
	b.mu.RUnlock()

	for _, s := range subs {
		if s.ordered {
			select {
			case s.queue <- dispatchItem{ctx: ctx, ev: ev}:
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		b.wg.Add(1)
		s := s
		go func() {
			b.sem <- struct{}{} // acquire worker slot
			defer func() {
				<-b.sem
				b.wg.Done()
			}()
			if err := s.h(ctx, ev); err != nil {
				slog.Error("eventbus: handler failed",
					"topic", ev.Topic(), "error", err)
			}
		}()
	}
	return nil
}

func (b *inproc) Subscribe(topic string, h Handler) func() {
	s := &subscription{id: b.nextID.Add(1), h: h}
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], s)
	b.mu.Unlock()
	return b.unsubFunc(topic, s)
}

func (b *inproc) SubscribeOrdered(topic string, h Handler) func() {
	q := make(chan dispatchItem, 64)
	s := &subscription{
		id: b.nextID.Add(1), h: h, ordered: true, queue: q,
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for item := range q {
			if err := h(item.ctx, item.ev); err != nil {
				slog.Error("eventbus: ordered handler failed",
					"topic", topic, "error", err)
			}
		}
	}()

	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], s)
	b.mu.Unlock()
	return b.unsubFunc(topic, s)
}

func (b *inproc) unsubFunc(topic string, target *subscription) func() {
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		list := b.subs[topic]
		for i, s := range list {
			if s.id == target.id {
				b.subs[topic] = append(list[:i], list[i+1:]...)
				target.closeOnce.Do(func() {
					if target.queue != nil {
						close(target.queue)
					}
				})
				return
			}
		}
	}
}

// Close stops accepting new publishes and waits for all in-flight handlers
// to finish, up to the given timeout.
func (b *inproc) Close(timeout time.Duration) error {
	b.closed.Store(true)

	b.mu.RLock()
	for _, list := range b.subs {
		for _, s := range list {
			s.closeOnce.Do(func() {
				if s.queue != nil {
					close(s.queue)
				}
			})
		}
	}
	b.mu.RUnlock()

	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return ErrDrainTimeout
	}
}

var (
	// ErrBusClosed is returned by Publish after Close has been called.
	ErrBusClosed = errBusClosed("eventbus: publish after close")
	// ErrDrainTimeout is returned by Close when handlers don't finish in time.
	ErrDrainTimeout = errDrainTimeout("eventbus: drain timeout")
)

type errBusClosed string

func (e errBusClosed) Error() string { return string(e) }

type errDrainTimeout string

func (e errDrainTimeout) Error() string { return string(e) }
