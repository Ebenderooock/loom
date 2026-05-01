// Package eventbus is an in-process, typed publish/subscribe bus used by
// Loom modules to communicate without taking direct dependencies on each
// other. The interface is defined to match what an embedded NATS backend
// will provide in split-mode (Phase 11), so downstream code is unchanged.
package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
)

// Event is anything that has a topic. Concrete events live in module
// packages (e.g. indexers.GrabRequested) so dependencies stay one-way.
type Event interface {
	Topic() string
}

// Handler is invoked once per published event of its subscribed topic.
// Handlers run in the publisher's goroutine; long work must be scheduled.
type Handler func(ctx context.Context, ev Event) error

// Bus is the public interface; tests can replace it freely.
type Bus interface {
	Publish(ctx context.Context, ev Event) error
	Subscribe(topic string, h Handler) (unsubscribe func())
}

type inproc struct {
	mu     sync.RWMutex
	subs   map[string][]subscription
	nextID atomic.Uint64
}

type subscription struct {
	id uint64
	h  Handler
}

// NewInProc returns an in-process bus. Safe for concurrent use.
func NewInProc() Bus {
	return &inproc{subs: map[string][]subscription{}}
}

func (b *inproc) Publish(ctx context.Context, ev Event) error {
	b.mu.RLock()
	subs := append([]subscription(nil), b.subs[ev.Topic()]...)
	b.mu.RUnlock()
	for _, s := range subs {
		if err := s.h(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

func (b *inproc) Subscribe(topic string, h Handler) func() {
	id := b.nextID.Add(1)
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], subscription{id: id, h: h})
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		list := b.subs[topic]
		for i, s := range list {
			if s.id == id {
				b.subs[topic] = append(list[:i], list[i+1:]...)
				return
			}
		}
	}
}
