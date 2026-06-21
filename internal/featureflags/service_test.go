package featureflags

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE feature_flags (
		key TEXT PRIMARY KEY,
		enabled INTEGER NOT NULL,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	svc := NewService(NewStore(db), nil)
	svc.Load(context.Background())
	return svc, db
}

func TestEnabledUsesRegistryDefault(t *testing.T) {
	svc, db := newTestService(t)
	defer db.Close()

	if !svc.Enabled(KeySearchLog) {
		t.Fatalf("expected search_log to default enabled")
	}
	if svc.Enabled("does_not_exist") {
		t.Fatalf("unknown flag should be disabled")
	}
}

func TestSetPersistsAndCaches(t *testing.T) {
	svc, db := newTestService(t)
	defer db.Close()

	if err := svc.Set(context.Background(), KeySearchLog, false); err != nil {
		t.Fatalf("set: %v", err)
	}
	if svc.Enabled(KeySearchLog) {
		t.Fatalf("expected search_log disabled after Set")
	}

	// A fresh service loading from the same DB must see the override.
	svc2 := NewService(NewStore(db), nil)
	svc2.Load(context.Background())
	if svc2.Enabled(KeySearchLog) {
		t.Fatalf("expected override to persist across reload")
	}
}

func TestSetUnknownFlagRejected(t *testing.T) {
	svc, db := newTestService(t)
	defer db.Close()

	if err := svc.Set(context.Background(), "bogus", true); err == nil {
		t.Fatalf("expected error setting unknown flag")
	}
}

func TestListReportsEffectiveValues(t *testing.T) {
	svc, db := newTestService(t)
	defer db.Close()

	_ = svc.Set(context.Background(), KeySearchLog, false)
	flags := svc.List()
	if len(flags) != len(Definitions) {
		t.Fatalf("expected %d flags, got %d", len(Definitions), len(flags))
	}
	var found bool
	for _, f := range flags {
		if f.Key == KeySearchLog {
			found = true
			if f.Enabled {
				t.Fatalf("expected search_log effective value false")
			}
		}
	}
	if !found {
		t.Fatalf("search_log not present in List")
	}
}

func TestEnabledFuncReflectsLatest(t *testing.T) {
	svc, db := newTestService(t)
	defer db.Close()

	fn := svc.EnabledFunc(KeySearchLog)
	if !fn() {
		t.Fatalf("expected enabled initially")
	}
	_ = svc.Set(context.Background(), KeySearchLog, false)
	if fn() {
		t.Fatalf("expected EnabledFunc to reflect disabled state")
	}
}
