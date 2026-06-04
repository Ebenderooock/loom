package bots

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeTransport struct {
	started  atomic.Int32
	stopped  atomic.Int32
	token    string
	mu       sync.Mutex
	lastErr  string
}

func (f *fakeTransport) Run(ctx context.Context) error {
	f.started.Add(1)
	<-ctx.Done()
	f.stopped.Add(1)
	return ctx.Err()
}
func (f *fakeTransport) LastError() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastErr
}

func TestSupervisor_StartStopOnConfig(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	var tgTransports []*fakeTransport
	var dcTransports []*fakeTransport
	tgFactory := func(token string) Transport {
		ft := &fakeTransport{token: token}
		tgTransports = append(tgTransports, ft)
		return ft
	}
	dcFactory := func(token string) Transport {
		ft := &fakeTransport{token: token}
		dcTransports = append(dcTransports, ft)
		return ft
	}

	sup := NewSupervisor(st, tgFactory, dcFactory, nil)
	if err := sup.Start(ctx); err != nil {
		t.Fatal(err)
	}
	// Nothing enabled yet.
	if len(tgTransports) != 0 || len(dcTransports) != 0 {
		t.Fatalf("expected no transports, got tg=%d dc=%d", len(tgTransports), len(dcTransports))
	}

	// Enable telegram.
	cfg, _ := st.GetConfig(ctx)
	cfg.TelegramEnabled = true
	cfg.TelegramBotToken = "tok1"
	st.SetConfig(ctx, cfg)
	if err := sup.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, func() bool { return len(tgTransports) == 1 && tgTransports[0].started.Load() == 1 })

	// Reload with same token: no restart.
	if err := sup.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if len(tgTransports) != 1 {
		t.Fatalf("expected no restart on unchanged token, got %d", len(tgTransports))
	}

	// Change token: old stops, new starts.
	cfg.TelegramBotToken = "tok2"
	st.SetConfig(ctx, cfg)
	if err := sup.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, func() bool { return len(tgTransports) == 2 && tgTransports[1].started.Load() == 1 })
	if tgTransports[0].stopped.Load() != 1 {
		t.Fatal("expected old telegram transport to be stopped")
	}

	// Disable telegram.
	cfg.TelegramEnabled = false
	st.SetConfig(ctx, cfg)
	if err := sup.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if tgTransports[1].stopped.Load() != 1 {
		t.Fatal("expected telegram transport stopped on disable")
	}

	sup.Shutdown()
}

func TestSupervisor_StatusReflectsRunning(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	sup := NewSupervisor(st,
		func(token string) Transport { return &fakeTransport{token: token} },
		func(token string) Transport { return &fakeTransport{token: token} }, nil)
	cfg, _ := st.GetConfig(ctx)
	cfg.DiscordEnabled = true
	cfg.DiscordBotToken = "d"
	st.SetConfig(ctx, cfg)
	_ = sup.Start(ctx)
	defer sup.Shutdown()

	statuses := sup.Status()
	var dc PlatformStatus
	for _, s := range statuses {
		if s.Platform == PlatformDiscord {
			dc = s
		}
	}
	if !dc.Running {
		t.Fatal("expected discord running")
	}
}

func waitStarted(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if cond() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("condition not met in time")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
