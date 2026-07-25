package notifications

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openNotificationsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
CREATE TABLE notification_connections (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	enabled BOOLEAN DEFAULT true,
	settings TEXT DEFAULT '{}',
	on_grab BOOLEAN DEFAULT false,
	on_download BOOLEAN DEFAULT false,
	on_upgrade BOOLEAN DEFAULT false,
	on_rename BOOLEAN DEFAULT false,
	on_delete BOOLEAN DEFAULT false,
	on_health_issue BOOLEAN DEFAULT false,
	on_application_update BOOLEAN DEFAULT false,
	on_playback BOOLEAN DEFAULT false,
	tags TEXT DEFAULT '[]',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE notification_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	connection_id TEXT REFERENCES notification_connections(id) ON DELETE SET NULL,
	event_type TEXT NOT NULL,
	title TEXT NOT NULL,
	message TEXT DEFAULT '',
	success BOOLEAN DEFAULT true,
	error_message TEXT DEFAULT '',
	sent_at TEXT NOT NULL
);`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func makeTestConnection(id, name string) *Connection {
	return &Connection{
		ID:      id,
		Name:    name,
		Type:    ConnectionTypeWebhook,
		Enabled: true,
		Settings: ConnectionSettings{
			WebhookURL: "https://example.test/hook",
		},
		OnDownload: true,
		Tags:       []string{"movies"},
	}
}

func TestServiceConnectionCRUD(t *testing.T) {
	t.Parallel()
	db := openNotificationsTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	c := makeTestConnection("", "Primary")
	if err := svc.CreateConnection(ctx, c); err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}
	if c.ID == "" {
		t.Fatal("CreateConnection should generate ID when empty")
	}

	got, err := svc.GetConnection(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetConnection: %v", err)
	}
	if got.Name != "Primary" || got.Type != ConnectionTypeWebhook {
		t.Fatalf("unexpected connection: %+v", got)
	}
	if got.Settings.WebhookURL != "https://example.test/hook" {
		t.Fatalf("settings not persisted: %+v", got.Settings)
	}

	got.Name = "Primary Updated"
	got.Enabled = false
	got.OnPlayback = true
	got.Tags = []string{"movies", "manual"}
	if err := svc.UpdateConnection(ctx, got); err != nil {
		t.Fatalf("UpdateConnection: %v", err)
	}

	updated, err := svc.GetConnection(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetConnection after update: %v", err)
	}
	if updated.Name != "Primary Updated" || updated.Enabled {
		t.Fatalf("update not applied: %+v", updated)
	}
	if !updated.OnPlayback || len(updated.Tags) != 2 {
		t.Fatalf("flags/tags not updated: %+v", updated)
	}

	if err := svc.DeleteConnection(ctx, got.ID); err != nil {
		t.Fatalf("DeleteConnection: %v", err)
	}
	if _, err := svc.GetConnection(ctx, got.ID); err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestServiceListConnectionsAndErrors(t *testing.T) {
	t.Parallel()
	db := openNotificationsTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	for _, tc := range []struct {
		id   string
		name string
	}{
		{"b", "Beta"},
		{"a", "Alpha"},
	} {
		c := makeTestConnection(tc.id, tc.name)
		if err := svc.CreateConnection(ctx, c); err != nil {
			t.Fatalf("CreateConnection(%s): %v", tc.name, err)
		}
	}

	list, err := svc.ListConnections(ctx)
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(list))
	}
	if list[0].Name != "Alpha" || list[1].Name != "Beta" {
		t.Fatalf("connections should be sorted by name: %+v", list)
	}

	if _, err := svc.GetConnection(ctx, "missing"); err == nil {
		t.Fatal("expected GetConnection error for missing id")
	}
	if err := svc.UpdateConnection(ctx, makeTestConnection("missing", "x")); err == nil {
		t.Fatal("expected UpdateConnection error for missing id")
	}
	if err := svc.DeleteConnection(ctx, "missing"); err == nil {
		t.Fatal("expected DeleteConnection error for missing id")
	}
}

func TestServiceLogHistoryAndListHistory(t *testing.T) {
	t.Parallel()
	db := openNotificationsTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	c := makeTestConnection("c1", "History")
	if err := svc.CreateConnection(ctx, c); err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}

	for i := 0; i < 60; i++ {
		success := i%2 == 0
		errMsg := ""
		if !success {
			errMsg = "failed"
		}
		if err := svc.LogHistory(ctx, &c.ID, string(EventOnDownload), "Download", "Completed", success, errMsg); err != nil {
			t.Fatalf("LogHistory(%d): %v", i, err)
		}
	}

	defaultLimited, err := svc.ListHistory(ctx, 0)
	if err != nil {
		t.Fatalf("ListHistory default limit: %v", err)
	}
	if len(defaultLimited) != 50 {
		t.Fatalf("expected default limit 50, got %d", len(defaultLimited))
	}

	limited, err := svc.ListHistory(ctx, 5)
	if err != nil {
		t.Fatalf("ListHistory(5): %v", err)
	}
	if len(limited) != 5 {
		t.Fatalf("expected 5 history entries, got %d", len(limited))
	}
	for _, h := range limited {
		if h.ConnectionID == nil || *h.ConnectionID != c.ID {
			t.Fatalf("unexpected connection_id in history: %+v", h)
		}
		if h.EventType != string(EventOnDownload) {
			t.Fatalf("unexpected event type: %+v", h)
		}
	}
}
