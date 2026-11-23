//go:build linux || darwin

package updater

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotFailureCannotRestartWithAnUnsettledJournal(t *testing.T) {
	e, s, _ := testEngine(t)
	s.failSnapshot = true
	path := filepath.Join(e.paths.state, "journal.json")
	s.beforeSnapshot = func() {
		if err := os.WriteFile(path, []byte("{incomplete"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.run(context.Background()); err == nil {
		t.Fatal("snapshot/recovery failure reported success")
	}
	if s.running || s.starts != 0 {
		t.Fatal("resumed application writes with an unsettled journal")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "{incomplete" {
		t.Fatal("unsettled journal was discarded", err)
	}
	raw, err = os.ReadFile(filepath.Join(e.paths.bin, "loom"))
	if err != nil || string(raw) != "old" {
		t.Fatal("binary changed before a complete snapshot", err)
	}
}
