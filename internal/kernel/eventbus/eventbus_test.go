package eventbus

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

type fakeEvent struct{ name string }

func (f fakeEvent) Topic() string { return f.name }

func TestPublishDeliversToSubscribers(t *testing.T) {
	b := NewInProc()
	var hits atomic.Int32
	unsub := b.Subscribe("a", func(ctx context.Context, ev Event) error {
		hits.Add(1)
		return nil
	})
	defer unsub()

	if err := b.Publish(context.Background(), fakeEvent{"a"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Publish(context.Background(), fakeEvent{"b"}); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("hits = %d, want 1", got)
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := NewInProc()
	var hits atomic.Int32
	unsub := b.Subscribe("a", func(ctx context.Context, ev Event) error {
		hits.Add(1)
		return nil
	})
	unsub()
	if err := b.Publish(context.Background(), fakeEvent{"a"}); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("after unsubscribe, hits = %d, want 0", got)
	}
}

func TestPublishPropagatesError(t *testing.T) {
	b := NewInProc()
	want := errors.New("boom")
	b.Subscribe("a", func(ctx context.Context, ev Event) error { return want })
	if err := b.Publish(context.Background(), fakeEvent{"a"}); !errors.Is(err, want) {
		t.Errorf("Publish err = %v, want %v", err, want)
	}
}

func TestPublishWithNoSubscribers(t *testing.T) {
	b := NewInProc()
	// Publishing to a topic with no subscribers should succeed silently
	if err := b.Publish(context.Background(), fakeEvent{"unused"}); err != nil {
		t.Errorf("Publish to empty topic: %v", err)
	}
}

func TestMultipleSubscribersSameTopic(t *testing.T) {
	b := NewInProc()
	var h1, h2 atomic.Int32

	unsub1 := b.Subscribe("a", func(ctx context.Context, ev Event) error {
		h1.Add(1)
		return nil
	})
	unsub2 := b.Subscribe("a", func(ctx context.Context, ev Event) error {
		h2.Add(1)
		return nil
	})
	defer unsub1()
	defer unsub2()

	if err := b.Publish(context.Background(), fakeEvent{"a"}); err != nil {
		t.Fatal(err)
	}
	if got := h1.Load(); got != 1 {
		t.Errorf("handler1 hits = %d, want 1", got)
	}
	if got := h2.Load(); got != 1 {
		t.Errorf("handler2 hits = %d, want 1", got)
	}
}

func TestUnsubscribeOnlyAffectsTarget(t *testing.T) {
	b := NewInProc()
	var h1, h2 atomic.Int32

	unsub1 := b.Subscribe("a", func(ctx context.Context, ev Event) error {
		h1.Add(1)
		return nil
	})
	b.Subscribe("a", func(ctx context.Context, ev Event) error {
		h2.Add(1)
		return nil
	})

	// Unsubscribe only the first handler
	unsub1()

	if err := b.Publish(context.Background(), fakeEvent{"a"}); err != nil {
		t.Fatal(err)
	}
	if got := h1.Load(); got != 0 {
		t.Errorf("handler1 hits = %d, want 0 (unsubscribed)", got)
	}
	if got := h2.Load(); got != 1 {
		t.Errorf("handler2 hits = %d, want 1 (still subscribed)", got)
	}
}

func TestMultipleTopicsIndependent(t *testing.T) {
	b := NewInProc()
	var hitsA, hitsB atomic.Int32

	b.Subscribe("a", func(ctx context.Context, ev Event) error {
		hitsA.Add(1)
		return nil
	})
	b.Subscribe("b", func(ctx context.Context, ev Event) error {
		hitsB.Add(1)
		return nil
	})

	b.Publish(context.Background(), fakeEvent{"a"})
	b.Publish(context.Background(), fakeEvent{"b"})
	b.Publish(context.Background(), fakeEvent{"b"})

	if got := hitsA.Load(); got != 1 {
		t.Errorf("topic A hits = %d, want 1", got)
	}
	if got := hitsB.Load(); got != 2 {
		t.Errorf("topic B hits = %d, want 2", got)
	}
}
