package nzbget

import (
	"context"
	"errors"
	"testing"
)

func TestPause_All(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("pausedownload", true)

	c := newTestClient(t, f)
	if err := c.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if got := f.callCount("pausedownload"); got != 1 {
		t.Fatalf("pausedownload calls = %d", got)
	}
	if f.callCount("editqueue") != 0 {
		t.Fatal("editqueue should not be called for pause-all")
	}
}

func TestResume_All(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("resumedownload", true)

	c := newTestClient(t, f)
	if err := c.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if f.callCount("resumedownload") != 1 {
		t.Fatalf("resumedownload calls = %d", f.callCount("resumedownload"))
	}
}

func TestPauseResume_PerID(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("editqueue", true)

	c := newTestClient(t, f)
	if err := c.Pause(context.Background(), "11", "12"); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	params := f.params("editqueue")
	if got := params[0].(string); got != "GroupPause" {
		t.Errorf("command = %q", got)
	}
	ids := params[3].([]any)
	if len(ids) != 2 {
		t.Errorf("expected 2 ids, got %d", len(ids))
	}

	if err := c.Resume(context.Background(), "11"); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got := f.params("editqueue")[0].(string); got != "GroupResume" {
		t.Errorf("resume command = %q", got)
	}
}

func TestRemove_DeleteFilesFalseUsesGroupDelete(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("editqueue", true)

	c := newTestClient(t, f)
	if err := c.Remove(context.Background(), []string{"99"}, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := f.params("editqueue")[0].(string); got != "GroupDelete" {
		t.Errorf("command = %q want GroupDelete", got)
	}
}

func TestRemove_DeleteFilesTrueUsesGroupFinalDelete(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("editqueue", true)

	c := newTestClient(t, f)
	if err := c.Remove(context.Background(), []string{"99"}, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := f.params("editqueue")[0].(string); got != "GroupFinalDelete" {
		t.Errorf("command = %q want GroupFinalDelete", got)
	}
}

func TestRemove_RefusalIsErrNotFound(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("editqueue", false)

	c := newTestClient(t, f)
	err := c.Remove(context.Background(), []string{"42"}, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRemove_NoIDsIsNoCall(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()

	c := newTestClient(t, f)
	if err := c.Remove(context.Background(), nil, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if f.callCount("editqueue") != 0 {
		t.Fatal("expected no editqueue call for empty ids")
	}
}

func TestRemove_NonNumericIDIsTyped(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()

	c := newTestClient(t, f)
	err := c.Remove(context.Background(), []string{"abc"}, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-numeric, got %v", err)
	}
}
