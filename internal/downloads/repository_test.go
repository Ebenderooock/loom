package downloads_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

func TestSQLiteRepositoryCRUD(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	repo := downloads.NewSQLiteRepository(raw)
	ctx := context.Background()

	def := downloads.Definition{
		ID:              "null-1",
		Name:            "Null One",
		Kind:            downloads.KindNull,
		Protocol:        downloads.ProtocolTorrent,
		Enabled:         true,
		Priority:        25,
		Host:            "localhost",
		Port:            8080,
		TLS:             true,
		Username:        "loom",
		Password:        "secret",
		Config:          json.RawMessage(`{"hello":"world"}`),
		CategoryDefault: "movies",
		SavePathDefault: "/downloads",
		RemoveCompleted: true,
		RemoveFailed:    false,
	}
	saved, err := repo.Create(ctx, def)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if saved.ID != "null-1" || saved.Kind != downloads.KindNull || saved.Protocol != downloads.ProtocolTorrent {
		t.Fatalf("Create returned wrong row: %#v", saved)
	}
	if !saved.Enabled || !saved.TLS || !saved.RemoveCompleted || saved.RemoveFailed {
		t.Fatalf("bool round-trip wrong: %#v", saved)
	}
	if saved.Username != "loom" || saved.Password != "secret" {
		t.Fatalf("creds round-trip wrong: %q/%q", saved.Username, saved.Password)
	}

	got, err := repo.Get(ctx, "null-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Null One" || got.Port != 8080 {
		t.Fatalf("Get bad row: %#v", got)
	}

	all, err := repo.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("List: %v len=%d", err, len(all))
	}

	enabled, err := repo.ListEnabled(ctx)
	if err != nil || len(enabled) != 1 {
		t.Fatalf("ListEnabled: %v len=%d", err, len(enabled))
	}

	// Patch toggle Enabled false; ListEnabled drops to zero.
	disabled := false
	if _, err := repo.Patch(ctx, downloads.Patch{ID: "null-1", Enabled: &disabled}); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	enabled, _ = repo.ListEnabled(ctx)
	if len(enabled) != 0 {
		t.Fatalf("expected ListEnabled empty after Patch, got %d", len(enabled))
	}

	// Replace overwrites everything.
	def2 := saved
	def2.Name = "Renamed"
	def2.Enabled = true
	def2.RemoveFailed = true
	if _, err := repo.Replace(ctx, def2); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	got, _ = repo.Get(ctx, "null-1")
	if got.Name != "Renamed" || !got.RemoveFailed {
		t.Fatalf("Replace did not stick: %#v", got)
	}

	if err := repo.Delete(ctx, "null-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(ctx, "null-1"); !errors.Is(err, downloads.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteRepositoryHealthRoundTrip(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	repo := downloads.NewSQLiteRepository(raw)
	ctx := context.Background()

	if _, err := repo.Create(ctx, downloads.Definition{
		ID: "n1", Name: "n1", Kind: downloads.KindNull, Protocol: downloads.ProtocolTorrent, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	bytes := int64(1024 * 1024 * 1024)
	h := downloads.Health{
		ClientID:            "n1",
		Status:              downloads.StatusOK,
		ConsecutiveFailures: 3,
		LastFreeSpaceBytes:  &bytes,
		LastCategories: []downloads.Category{
			{Name: "movies", SavePath: "/m"},
			{Name: "tv", SavePath: "/t"},
		},
	}
	if err := repo.UpsertHealth(ctx, h); err != nil {
		t.Fatalf("UpsertHealth: %v", err)
	}
	got, err := repo.GetHealth(ctx, "n1")
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	if got.Status != downloads.StatusOK || got.ConsecutiveFailures != 3 {
		t.Fatalf("Health round-trip mismatch: %#v", got)
	}
	if got.LastFreeSpaceBytes == nil || *got.LastFreeSpaceBytes != bytes {
		t.Fatalf("free-space round-trip wrong: %#v", got.LastFreeSpaceBytes)
	}
	if len(got.LastCategories) != 2 || got.LastCategories[1].Name != "tv" {
		t.Fatalf("categories round-trip wrong: %#v", got.LastCategories)
	}

	all, err := repo.ListHealth(ctx)
	if err != nil || len(all) != 1 || all["n1"].Status != downloads.StatusOK {
		t.Fatalf("ListHealth: %v %#v", err, all)
	}
}
