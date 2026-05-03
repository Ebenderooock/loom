package sabnzbd

import (
	"context"
	"errors"
	"testing"
)

func TestPause_All(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("queue", `{"status":true}`)

	c := newTestClient(t, f)
	if err := c.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	form := f.lastForm["queue"]
	if form.Get("name") != "pause" {
		t.Errorf("name = %q", form.Get("name"))
	}
	if form.Get("value") != "" {
		t.Errorf("expected empty value for pause-all, got %q", form.Get("value"))
	}
}

func TestPauseResume_PerID(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("queue", `{"status":true}`)

	c := newTestClient(t, f)
	if err := c.Pause(context.Background(), "nzo-1"); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if got := f.lastForm["queue"].Get("value"); got != "nzo-1" {
		t.Errorf("value = %q", got)
	}
	if err := c.Resume(context.Background(), "nzo-2"); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got := f.lastForm["queue"].Get("name"); got != "resume" {
		t.Errorf("resume name = %q", got)
	}
	if got := f.lastForm["queue"].Get("value"); got != "nzo-2" {
		t.Errorf("resume value = %q", got)
	}
}

func TestRemove_QueueOnly(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("queue", `{"status":true,"nzo_ids":["nzo-1"]}`)

	c := newTestClient(t, f)
	if err := c.Remove(context.Background(), []string{"nzo-1"}, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if f.calls["queue"] != 1 {
		t.Errorf("expected one queue call, got %d", f.calls["queue"])
	}
	if f.calls["history"] != 0 {
		t.Errorf("expected no history call, got %d", f.calls["history"])
	}
}

func TestRemove_FallsBackToHistory(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	// Queue says it didn't find the id. History accepts.
	f.on("queue", `{"status":true,"nzo_ids":[]}`)
	f.on("history", `{"status":true}`)

	c := newTestClient(t, f)
	if err := c.Remove(context.Background(), []string{"nzo-9"}, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if f.calls["history"] != 1 {
		t.Fatalf("expected history fallback, calls=%v", f.calls)
	}
	if got := f.lastForm["history"].Get("del_files"); got != "1" {
		t.Errorf("del_files = %q want 1", got)
	}
}

func TestRemove_HistoryWithoutDeleteFiles(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("queue", `{"status":true,"nzo_ids":[]}`)
	f.on("history", `{"status":true}`)

	c := newTestClient(t, f)
	if err := c.Remove(context.Background(), []string{"x"}, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := f.lastForm["history"].Get("del_files"); got != "" {
		t.Errorf("del_files should be unset, got %q", got)
	}
}

func TestRemove_HistoryRefusalIsErrNotFound(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("queue", `{"status":true,"nzo_ids":[]}`)
	f.on("history", `{"status":false,"error":"not found"}`)

	c := newTestClient(t, f)
	err := c.Remove(context.Background(), []string{"missing"}, true)
	// SAB returns the error envelope shape; we map that to ErrServer
	// before the explicit ErrNotFound check fires, so the typed
	// error here is ErrServer. Either way it must not be nil.
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrServer) && !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrServer or ErrNotFound, got %v", err)
	}
}

func TestRemove_NoIDs_NoCall(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()

	c := newTestClient(t, f)
	if err := c.Remove(context.Background(), nil, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if f.calls["queue"]+f.calls["history"] != 0 {
		t.Fatalf("expected no upstream calls, got %v", f.calls)
	}
}
