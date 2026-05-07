package qbittorrent

import (
	"context"
	"net/http"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

func TestPauseAllAndSpecific(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	var got string
	f.mux.HandleFunc("/api/v2/torrents/pause", f.requireSID(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.PostFormValue("hashes")
		w.WriteHeader(http.StatusOK)
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}

	if err := c.Pause(context.Background()); err != nil {
		t.Fatalf("Pause(all): %v", err)
	}
	if got != "all" {
		t.Errorf("Pause() hashes = %q, want %q", got, "all")
	}

	if err := c.Pause(context.Background(), "h1", "h2"); err != nil {
		t.Fatalf("Pause(h1,h2): %v", err)
	}
	if got != "h1|h2" {
		t.Errorf("Pause hashes = %q, want %q", got, "h1|h2")
	}
}

func TestResume(t *testing.T) {
	t.Parallel()
	f := newFakeServer("adminadmin")
	defer f.Close()
	called := false
	f.mux.HandleFunc("/api/v2/torrents/resume", f.requireSID(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = r.ParseForm()
		if r.PostFormValue("hashes") != "abc" {
			t.Errorf("hashes = %q", r.PostFormValue("hashes"))
		}
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := c.Resume(context.Background(), "abc"); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !called {
		t.Fatal("resume handler not called")
	}
}

func TestRemoveDeleteFilesTrue(t *testing.T) {
	t.Parallel()
	checkRemove(t, true)
}

func TestRemoveDeleteFilesFalse(t *testing.T) {
	t.Parallel()
	checkRemove(t, false)
}

func checkRemove(t *testing.T, deleteFiles bool) {
	t.Helper()
	f := newFakeServer("adminadmin")
	defer f.Close()
	var saw string
	f.mux.HandleFunc("/api/v2/torrents/delete", f.requireSID(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		saw = r.PostFormValue("deleteFiles")
		if r.PostFormValue("hashes") != "x|y" {
			t.Errorf("hashes = %q", r.PostFormValue("hashes"))
		}
	}))

	c := newTestClient(t, f.srv, downloads.Definition{})
	if err := c.login(context.Background(), true); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := c.Remove(context.Background(), []string{"x", "y"}, deleteFiles); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := "false"
	if deleteFiles {
		want = "true"
	}
	if saw != want {
		t.Errorf("deleteFiles = %q, want %q", saw, want)
	}
}
