package downloads_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

func newServiceForTest(t *testing.T) *downloads.Service {
	t.Helper()
	_, raw := openTestDB(t)
	repo := downloads.NewSQLiteRepository(raw)
	svc, err := downloads.NewService(downloads.ServiceOptions{
		Repository: repo,
		Logger:     quietLogger(),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestServiceCreateHydratesAndSeedsHealth(t *testing.T) {
	t.Parallel()
	svc := newServiceForTest(t)
	ctx := context.Background()

	saved, err := svc.Create(ctx, downloads.Definition{
		ID:       "n1",
		Name:     "Null",
		Kind:     downloads.KindNull,
		Protocol: downloads.ProtocolTorrent,
		Enabled:  true,
		Priority: 10,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if saved.ID != "n1" {
		t.Fatalf("Create returned wrong: %#v", saved)
	}
	if svc.Registry().Len() != 1 {
		t.Fatalf("registry not hydrated, len=%d", svc.Registry().Len())
	}
	dh, err := svc.GetWithHealth(ctx, "n1")
	if err != nil {
		t.Fatalf("GetWithHealth: %v", err)
	}
	if dh.Health == nil || dh.Health.Status != downloads.StatusUnknown {
		t.Fatalf("expected unknown initial health: %#v", dh.Health)
	}
}

func TestServiceCreateRejectsUnknownKind(t *testing.T) {
	t.Parallel()
	svc := newServiceForTest(t)
	_, err := svc.Create(context.Background(), downloads.Definition{
		ID: "x", Name: "x", Kind: "not-a-kind", Protocol: downloads.ProtocolTorrent,
	})
	if !errors.Is(err, downloads.ErrUnknownKind) {
		t.Fatalf("expected ErrUnknownKind, got %v", err)
	}
}

func TestServiceTestOnePersistsHealth(t *testing.T) {
	t.Parallel()
	svc := newServiceForTest(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, downloads.Definition{
		ID: "n1", Name: "Null", Kind: downloads.KindNull, Protocol: downloads.ProtocolTorrent, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	h, err := svc.TestOne(ctx, "n1")
	if err != nil {
		t.Fatalf("TestOne: %v", err)
	}
	if h.Status != downloads.StatusOK {
		t.Fatalf("expected ok, got %s", h.Status)
	}
	// LastFreeSpaceBytes is nil because nullClient returns -1.
	if h.LastFreeSpaceBytes != nil {
		t.Fatalf("expected nil free-space for null client, got %v", *h.LastFreeSpaceBytes)
	}

	// Persisted?
	dh, _ := svc.GetWithHealth(ctx, "n1")
	if dh.Health == nil || dh.Health.Status != downloads.StatusOK {
		t.Fatalf("health not persisted: %#v", dh.Health)
	}
}

func TestServiceTestOneNotFound(t *testing.T) {
	t.Parallel()
	svc := newServiceForTest(t)
	if _, err := svc.TestOne(context.Background(), "missing"); !errors.Is(err, downloads.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestServicePatchAndDelete(t *testing.T) {
	t.Parallel()
	svc := newServiceForTest(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, downloads.Definition{
		ID: "n1", Name: "Null", Kind: downloads.KindNull, Protocol: downloads.ProtocolTorrent, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	disabled := false
	saved, err := svc.Patch(ctx, downloads.Patch{ID: "n1", Enabled: &disabled})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if saved.Enabled {
		t.Fatalf("expected disabled")
	}
	if _, ok := svc.Registry().Get("n1"); ok {
		t.Fatalf("disabled client should be removed from registry")
	}

	if err := svc.Delete(ctx, "n1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, "n1"); !errors.Is(err, downloads.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete: %v", err)
	}
}
