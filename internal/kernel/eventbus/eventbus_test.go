package eventbus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeEvent struct{ name string }

func (f fakeEvent) Topic() string { return f.name }

// helper: publish and wait for all async handlers to settle.
func publishAndDrain(t *testing.T, b AsyncBus, ctx context.Context, ev Event) {
	t.Helper()
	if err := b.Publish(ctx, ev); err != nil {
		t.Fatal(err)
	}
	// Give async goroutines time to start and complete.
	if err := b.Close(2 * time.Second); err != nil {
		t.Fatal(err)
	}
}

// newBus creates a fresh bus for each test.
func newBus() AsyncBus { return NewInProc() }

func TestPublishDeliversToSubscribers(t *testing.T) {
	b := newBus()
	var hits atomic.Int32
	done := make(chan struct{}, 1)
	unsub := b.Subscribe("a", func(ctx context.Context, ev Event) error {
		hits.Add(1)
		done <- struct{}{}
		return nil
	})
	defer unsub()

	if err := b.Publish(context.Background(), fakeEvent{"a"}); err != nil {
		t.Fatal(err)
	}
	<-done
	// Publish to a different topic — handler should not fire.
	if err := b.Publish(context.Background(), fakeEvent{"b"}); err != nil {
		t.Fatal(err)
	}
	// Small sleep to verify no extra delivery.
	time.Sleep(20 * time.Millisecond)
	if got := hits.Load(); got != 1 {
		t.Errorf("hits = %d, want 1", got)
	}
	b.Close(time.Second)
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := newBus()
	var hits atomic.Int32
	unsub := b.Subscribe("a", func(ctx context.Context, ev Event) error {
		hits.Add(1)
		return nil
	})
	unsub()
	if err := b.Publish(context.Background(), fakeEvent{"a"}); err != nil {
		t.Fatal(err)
	}
	b.Close(time.Second)
	if got := hits.Load(); got != 0 {
		t.Errorf("after unsubscribe, hits = %d, want 0", got)
	}
}

func TestHandlerErrorIsLogged(t *testing.T) {
	// With async dispatch, handler errors are logged, not returned.
	b := newBus()
	b.Subscribe("a", func(ctx context.Context, ev Event) error { return errors.New("boom") })
	// Publish should succeed even though handler errors.
	if err := b.Publish(context.Background(), fakeEvent{"a"}); err != nil {
		t.Errorf("Publish err = %v, want nil (errors are async-logged)", err)
	}
	b.Close(time.Second)
}

func TestPublishWithNoSubscribers(t *testing.T) {
	b := newBus()
	if err := b.Publish(context.Background(), fakeEvent{"unused"}); err != nil {
		t.Errorf("Publish to empty topic: %v", err)
	}
	b.Close(time.Second)
}

func TestMultipleSubscribersSameTopic(t *testing.T) {
	b := newBus()
	var h1, h2 atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)

	unsub1 := b.Subscribe("a", func(ctx context.Context, ev Event) error {
		h1.Add(1)
		wg.Done()
		return nil
	})
	unsub2 := b.Subscribe("a", func(ctx context.Context, ev Event) error {
		h2.Add(1)
		wg.Done()
		return nil
	})
	defer unsub1()
	defer unsub2()

	if err := b.Publish(context.Background(), fakeEvent{"a"}); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if got := h1.Load(); got != 1 {
		t.Errorf("handler1 hits = %d, want 1", got)
	}
	if got := h2.Load(); got != 1 {
		t.Errorf("handler2 hits = %d, want 1", got)
	}
	b.Close(time.Second)
}

func TestUnsubscribeOnlyAffectsTarget(t *testing.T) {
	b := newBus()
	var h1, h2 atomic.Int32
	done := make(chan struct{}, 1)

	unsub1 := b.Subscribe("a", func(ctx context.Context, ev Event) error {
		h1.Add(1)
		return nil
	})
	b.Subscribe("a", func(ctx context.Context, ev Event) error {
		h2.Add(1)
		done <- struct{}{}
		return nil
	})

	unsub1()

	if err := b.Publish(context.Background(), fakeEvent{"a"}); err != nil {
		t.Fatal(err)
	}
	<-done
	if got := h1.Load(); got != 0 {
		t.Errorf("handler1 hits = %d, want 0 (unsubscribed)", got)
	}
	if got := h2.Load(); got != 1 {
		t.Errorf("handler2 hits = %d, want 1 (still subscribed)", got)
	}
	b.Close(time.Second)
}

func TestMultipleTopicsIndependent(t *testing.T) {
	b := newBus()
	var hitsA, hitsB atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3) // 1 for topic A, 2 for topic B

	b.Subscribe("a", func(ctx context.Context, ev Event) error {
		hitsA.Add(1)
		wg.Done()
		return nil
	})
	b.Subscribe("b", func(ctx context.Context, ev Event) error {
		hitsB.Add(1)
		wg.Done()
		return nil
	})

	b.Publish(context.Background(), fakeEvent{"a"})
	b.Publish(context.Background(), fakeEvent{"b"})
	b.Publish(context.Background(), fakeEvent{"b"})

	wg.Wait()
	if got := hitsA.Load(); got != 1 {
		t.Errorf("topic A hits = %d, want 1", got)
	}
	if got := hitsB.Load(); got != 2 {
		t.Errorf("topic B hits = %d, want 2", got)
	}
	b.Close(time.Second)
}

// ── New tests for async features ──

func TestConcurrentHandlerExecution(t *testing.T) {
	b := newBus()
	barrier := make(chan struct{})
	var running atomic.Int32
	var maxConcurrent atomic.Int32

	const n = 4
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		b.Subscribe("c", func(ctx context.Context, ev Event) error {
			cur := running.Add(1)
			// Track peak concurrency.
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			<-barrier
			running.Add(-1)
			wg.Done()
			return nil
		})
	}

	b.Publish(context.Background(), fakeEvent{"c"})
	// Let all handlers reach the barrier.
	time.Sleep(50 * time.Millisecond)
	close(barrier)
	wg.Wait()

	if peak := maxConcurrent.Load(); peak < 2 {
		t.Errorf("max concurrent handlers = %d, want >= 2 (async dispatch)", peak)
	}
	b.Close(time.Second)
}

func TestOrderedSubscription(t *testing.T) {
	b := newBus()
	var mu sync.Mutex
	var order []int

	const total = 20
	var wg sync.WaitGroup
	wg.Add(total)

	unsub := b.SubscribeOrdered("seq", func(ctx context.Context, ev Event) error {
		mu.Lock()
		order = append(order, ev.(indexedEvent).idx)
		mu.Unlock()
		time.Sleep(time.Millisecond) // simulate work
		wg.Done()
		return nil
	})
	defer unsub()

	for i := 0; i < total; i++ {
		b.Publish(context.Background(), indexedEvent{name: "seq", idx: i})
	}
	wg.Wait()

	for i, v := range order {
		if v != i {
			t.Fatalf("order[%d] = %d, want %d – ordered delivery violated", i, v, i)
		}
	}
	b.Close(time.Second)
}

type indexedEvent struct {
	name string
	idx  int
}

func (e indexedEvent) Topic() string { return e.name }

func TestCloseStopsPublish(t *testing.T) {
	b := newBus()
	b.Subscribe("x", func(ctx context.Context, ev Event) error { return nil })
	b.Close(time.Second)

	err := b.Publish(context.Background(), fakeEvent{"x"})
	if !errors.Is(err, ErrBusClosed) {
		t.Errorf("Publish after Close: err = %v, want ErrBusClosed", err)
	}
}

func TestCloseDrainsInFlight(t *testing.T) {
	b := newBus()
	started := make(chan struct{})
	var finished atomic.Bool

	b.Subscribe("slow", func(ctx context.Context, ev Event) error {
		close(started)
		time.Sleep(50 * time.Millisecond)
		finished.Store(true)
		return nil
	})

	b.Publish(context.Background(), fakeEvent{"slow"})
	<-started // handler is running

	if err := b.Close(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	if !finished.Load() {
		t.Error("Close returned before handler finished")
	}
}

func TestWithMaxWorkers(t *testing.T) {
	b := NewInProc(WithMaxWorkers(2))
	barrier := make(chan struct{})
	var running atomic.Int32

	const n = 4
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		b.Subscribe("w", func(ctx context.Context, ev Event) error {
			running.Add(1)
			<-barrier
			running.Add(-1)
			wg.Done()
			return nil
		})
	}

	b.Publish(context.Background(), fakeEvent{"w"})
	time.Sleep(50 * time.Millisecond)

	// With maxWorkers=2, only 2 should be running at once.
	if cur := running.Load(); cur > 2 {
		t.Errorf("running = %d with maxWorkers=2", cur)
	}
	close(barrier)
	wg.Wait()
	b.Close(time.Second)
}
