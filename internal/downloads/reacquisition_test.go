package downloads

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReacquisitionRecoveryFailureDoesNotRepeatClientAdd(t *testing.T) {
	ctx := context.Background()
	store := openHistoryTestStore(t)
	registry, client := acquisitionRegistry(t, store, false)
	req := AddRequest{Title: "Failed import", Infohash: strings.Repeat("b", 40)}
	if _, err := registry.Acquire(ctx, "client", req, nil); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List(ctx, 10, 0)
	if err != nil || len(entries) != 1 {
		t.Fatalf("history: %v, %v", entries, err)
	}
	if err := store.acquisitionAction(ctx, entries[0].ID, "retry"); err != nil {
		t.Fatal(err)
	}
	registry.SetReacquisitionHandler(func(context.Context, string, string) error {
		return errors.New("workflow storage unavailable")
	})
	if _, err := registry.Acquire(ctx, "client", req, nil); err == nil || !strings.Contains(err.Error(), "download accepted but import recovery failed") {
		t.Fatalf("recovery failure was not surfaced: %v", err)
	}
	if _, err := registry.Acquire(ctx, "client", req, nil); !errors.Is(err, ErrAcquisitionDeferred) {
		t.Fatalf("callback failure made a successful add automatically retryable: %v", err)
	}
	if client.calls.Load() != 2 {
		t.Fatalf("successful download was resubmitted: %d calls", client.calls.Load())
	}
}

func TestReacquisitionAuthorizationSurvivesIntermediateFailure(t *testing.T) {
	ctx := context.Background()
	store := openHistoryTestStore(t)
	registry, client := acquisitionRegistry(t, store, false)
	req := AddRequest{Title: "Recover import", Infohash: strings.Repeat("c", 40)}
	if _, err := registry.Acquire(ctx, "client", req, nil); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.acquisitionAction(ctx, entries[0].ID, "retry"); err != nil {
		t.Fatal(err)
	}
	recovered := false
	registry.SetReacquisitionHandler(func(context.Context, string, string) error {
		recovered = true
		return nil
	})
	client.fail = true
	if _, err := registry.Acquire(ctx, "client", req, nil); err == nil {
		t.Fatal("expected transient client failure")
	}
	if _, err := store.db.Exec(`UPDATE download_history SET next_retry_at = ''`); err != nil {
		t.Fatal(err)
	}
	client.fail = false
	if _, err := registry.Acquire(ctx, "client", req, nil); err != nil {
		t.Fatal(err)
	}
	if !recovered {
		t.Fatal("intermediate failure lost the explicit import recovery decision")
	}
}
