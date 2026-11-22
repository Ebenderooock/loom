package updates

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/notifications"
	"github.com/ebenderooock/loom/internal/storage"
)

// Alerts publishes update notifications and records which versions have been seen.
type Alerts struct {
	db  storage.DB
	bus eventbus.Bus
}

// NewAlerts connects persisted version state to the notification bus.
func NewAlerts(db storage.DB, bus eventbus.Bus) *Alerts {
	return &Alerts{db: db, bus: bus}
}

func (a *Alerts) query(query string) string {
	return storage.RebindQuery(query, a.db.Engine() == storage.EnginePostgres)
}

// Started records the running version; the first observed version is a baseline.
func (a *Alerts) Started(ctx context.Context, current string) error {
	if current == "" {
		return errors.New("cannot record an empty application version")
	}
	const key = "updates.last_seen_version"
	result, err := a.db.DB().ExecContext(ctx, a.query(
		`INSERT INTO schema_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO NOTHING`), key, current)
	if err != nil {
		return fmt.Errorf("record initial version: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record initial version: %w", err)
	}
	if inserted != 0 {
		return nil
	}
	for {
		var previous string
		err = a.db.DB().QueryRowContext(ctx, a.query(`SELECT value FROM schema_meta WHERE key = ?`), key).Scan(&previous)
		if err != nil {
			return fmt.Errorf("read previous version: %w", err)
		}
		if previous == current {
			return nil
		}
		result, err = a.db.DB().ExecContext(ctx, a.query(
			`UPDATE schema_meta SET value = ? WHERE key = ? AND value = ?`), current, key, previous)
		if err != nil {
			return fmt.Errorf("record running version: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("record running version: %w", err)
		}
		if changed == 0 {
			continue
		}
		message := fmt.Sprintf("%s -> %s", previous, current)
		err = a.bus.Publish(ctx, notifications.NotificationEvent{
			Event: notifications.EventOnApplicationUpdate, Title: "Loom updated", Message: message,
			Data: map[string]any{"title": message},
		})
		if err != nil {
			_, rollbackErr := a.db.DB().ExecContext(ctx, a.query(
				`UPDATE schema_meta SET value = ? WHERE key = ? AND value = ?`), previous, key, current)
			return errors.Join(fmt.Errorf("publish application update: %w", err), rollbackErr)
		}
		return nil
	}
}

// Available publishes at most one alert per verified, newer version.
func (a *Alerts) Available(ctx context.Context, notice Notice) error {
	if !notice.Verified || !notice.Available {
		return nil
	}
	if _, ok := parseVersion(notice.Version); !ok {
		return errors.New("verified update notice has an invalid version")
	}
	key := "updates.notified_version." + strings.TrimPrefix(notice.Version, "v")
	result, err := a.db.DB().ExecContext(ctx, a.query(
		`INSERT INTO schema_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO NOTHING`), key, notice.Version)
	if err != nil {
		return fmt.Errorf("record available version: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record available version: %w", err)
	}
	if inserted == 0 {
		return nil
	}
	message := fmt.Sprintf("%s -> %s: %s", notice.CurrentVersion, notice.Version, notice.URL)
	err = a.bus.Publish(ctx, notifications.NotificationEvent{
		Event: notifications.EventOnUpdateAvailable, Title: "Loom update available", Message: message,
		Data: map[string]any{"title": message},
	})
	if err != nil {
		_, rollbackErr := a.db.DB().ExecContext(ctx, a.query(
			`DELETE FROM schema_meta WHERE key = ?`), key)
		return errors.Join(fmt.Errorf("publish available update: %w", err), rollbackErr)
	}
	return nil
}
