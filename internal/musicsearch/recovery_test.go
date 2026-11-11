package musicsearch

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

func TestGrabFirstAvailableFindsRecoveryPastBlocklistedRelease(t *testing.T) {
	blocked := magnetCandidate("Blocked release", "0123456789abcdef0123456789abcdef01234567")
	retry := magnetCandidate("From Zero Deluxe 24bit", "89abcdef0123456789abcdef0123456789abcdef")
	registry, client := historyRegistry(t, blocked.Result.Infohash)
	engine := NewEngine(nil, registry, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := engine.grab(context.Background(), "album", &retry); err != nil {
		t.Fatal(err)
	}
	_, err := engine.grabFirstAvailable(context.Background(), "album", []Candidate{blocked, retry})
	var conflict *downloads.AcquisitionConflict
	if !errors.As(err, &conflict) || !conflict.RetryAllowed || conflict.Title != retry.Result.Title {
		t.Fatalf("blocklisted release hid an actionable recovery: %v", err)
	}
	if len(client.titles) != 1 {
		t.Fatalf("deferred search submitted another download: %v", client.titles)
	}
}
