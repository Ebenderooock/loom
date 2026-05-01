package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRegisterAndStop(t *testing.T) {
	s := New()
	var hits atomic.Int32
	s.Register(Task{
		Name:     "tick",
		Interval: 5 * time.Millisecond,
		Run: func(ctx context.Context) error {
			hits.Add(1)
			return nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	time.Sleep(40 * time.Millisecond)
	cancel()
	s.Stop()
	if hits.Load() == 0 {
		t.Errorf("expected at least one tick")
	}
}

func TestRegisterAfterStartPanics(t *testing.T) {
	s := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	defer s.Stop()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	s.Register(Task{Name: "x", Interval: time.Second, Run: func(context.Context) error { return nil }})
}
