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
