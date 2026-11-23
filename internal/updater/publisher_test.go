//go:build !windows

package updater

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/updates"
)

func TestMetadataBackoffSurvivesHelperRestart(t *testing.T) {
	dir := t.TempDir()
	next := map[string]time.Time{"stable": time.Now().Add(2 * time.Hour)}
	if err := atomicJSON(dir, "next-check.json", next); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		p := checkedPublisher{Checker: updates.NewChecker("dev"), stateDir: dir}
		if _, err := p.Discover(context.Background(), "stable"); err == nil || !strings.Contains(err.Error(), "throttled") {
			t.Fatalf("persisted backoff ignored: %v", err)
		}
	}
}

func TestRuntimeIdentityMustMatchSignedSourceAndChannel(t *testing.T) {
	a := Identity{Version: "1.2.3", SHA: strings.Repeat("a", 40), Ref: "refs/tags/v1.2.3", Channel: "stable"}
	b := a
	b.Version = "v1.2.3"
	if !sameIdentity(a, b) {
		t.Fatal("equivalent version prefix rejected")
	}
	b.Ref = "refs/heads/prelease/1.2.3"
	if sameIdentity(a, b) {
		t.Fatal("wrong source ref accepted")
	}
	b = a
	b.Channel = "prelease"
	if sameIdentity(a, b) {
		t.Fatal("wrong source channel accepted")
	}
}
