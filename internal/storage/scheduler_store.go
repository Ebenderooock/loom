package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/loomctl/loom/internal/kernel/scheduler"
	dbpg "github.com/loomctl/loom/internal/storage/db/postgres"
	dbsqlite "github.com/loomctl/loom/internal/storage/db/sqlite"
)

// NewSchedulerStore returns a scheduler.Store backed by db. It
// dispatches on db.Engine so callers don't need to care which
// sqlc-generated package is in play.
func NewSchedulerStore(db DB) scheduler.Store {
	switch db.Engine() {
	case EnginePostgres:
		return &pgSchedulerStore{q: dbpg.New(db.DB())}
	default:
		return &sqliteSchedulerStore{q: dbsqlite.New(db.DB())}
	}
}

// --- SQLite adapter -------------------------------------------------

type sqliteSchedulerStore struct {
	q *dbsqlite.Queries
}

func (s *sqliteSchedulerStore) UpsertJob(ctx context.Context, name, schedule string, payload []byte, nextRun time.Time) error {
	if !json.Valid(payload) {
		return fmt.Errorf("scheduler store: payload for %q is not valid JSON", name)
	}
	_, err := s.q.UpsertScheduledJob(ctx, dbsqlite.UpsertScheduledJobParams{
		Name:      name,
		Schedule:  schedule,
		Payload:   string(payload),
		NextRunAt: sql.NullTime{Time: nextRun, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("upsert job %q: %w", name, err)
	}
	return nil
}

func (s *sqliteSchedulerStore) RecordRun(ctx context.Context, name string, ranAt, nextRun time.Time, status, errMsg string) error {
	err := s.q.RecordScheduledJobRun(ctx, dbsqlite.RecordScheduledJobRunParams{
		Name:       name,
		LastRunAt:  sql.NullTime{Time: ranAt, Valid: true},
		NextRunAt:  sql.NullTime{Time: nextRun, Valid: true},
		LastStatus: status,
		LastError:  errMsg,
	})
	if err != nil {
		return fmt.Errorf("record run %q: %w", name, err)
	}
	return nil
}

func (s *sqliteSchedulerStore) SetNextRun(ctx context.Context, name string, nextRun time.Time) error {
	err := s.q.SetScheduledJobNextRun(ctx, dbsqlite.SetScheduledJobNextRunParams{
		Name:      name,
		NextRunAt: sql.NullTime{Time: nextRun, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("set next_run %q: %w", name, err)
	}
	return nil
}

// --- Postgres adapter -----------------------------------------------

type pgSchedulerStore struct {
	q *dbpg.Queries
}

func (p *pgSchedulerStore) UpsertJob(ctx context.Context, name, schedule string, payload []byte, nextRun time.Time) error {
	if !json.Valid(payload) {
		return fmt.Errorf("scheduler store: payload for %q is not valid JSON", name)
	}
	_, err := p.q.UpsertScheduledJob(ctx, dbpg.UpsertScheduledJobParams{
		Name:      name,
		Schedule:  schedule,
		Payload:   json.RawMessage(payload),
		NextRunAt: sql.NullTime{Time: nextRun, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("upsert job %q: %w", name, err)
	}
	return nil
}

func (p *pgSchedulerStore) RecordRun(ctx context.Context, name string, ranAt, nextRun time.Time, status, errMsg string) error {
	err := p.q.RecordScheduledJobRun(ctx, dbpg.RecordScheduledJobRunParams{
		Name:       name,
		LastRunAt:  sql.NullTime{Time: ranAt, Valid: true},
		NextRunAt:  sql.NullTime{Time: nextRun, Valid: true},
		LastStatus: status,
		LastError:  errMsg,
	})
	if err != nil {
		return fmt.Errorf("record run %q: %w", name, err)
	}
	return nil
}

func (p *pgSchedulerStore) SetNextRun(ctx context.Context, name string, nextRun time.Time) error {
	err := p.q.SetScheduledJobNextRun(ctx, dbpg.SetScheduledJobNextRunParams{
		Name:      name,
		NextRunAt: sql.NullTime{Time: nextRun, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("set next_run %q: %w", name, err)
	}
	return nil
}
