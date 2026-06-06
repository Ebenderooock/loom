package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Tunables for run-history retention.
const (
	maxRunsPerPlugin = 200
	maxRunAge        = 30 * 24 * time.Hour
	maxTimeoutSecs   = 300
	defaultTimeout   = 30
	maxSourceBytes   = 256 * 1024 // JS plugin source cap
)

// Store is the persistence + validation layer for plugins and their run history.
type Store struct {
	db *sql.DB
}

// NewStore returns a sqlite-backed plugin store.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// List returns all plugins ordered by name.
func (s *Store) List(ctx context.Context) ([]*Plugin, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, enabled, source, events, env, timeout_secs, created_at, updated_at
		FROM plugins ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list plugins: %w", err)
	}
	defer rows.Close()

	var out []*Plugin
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns a single plugin by id.
func (s *Store) Get(ctx context.Context, id string) (*Plugin, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, enabled, source, events, env, timeout_secs, created_at, updated_at
		FROM plugins WHERE id = ?`, id)
	p, err := scanPlugin(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("plugin not found: %s", id)
		}
		return nil, err
	}
	return p, nil
}

// enabledForTopic returns enabled plugins subscribed to the given event key.
func (s *Store) enabledForTopic(ctx context.Context, eventKey string) ([]*Plugin, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []*Plugin
	for _, p := range all {
		if !p.Enabled {
			continue
		}
		for _, e := range p.Events {
			if e == eventKey {
				out = append(out, p)
				break
			}
		}
	}
	return out, nil
}

// Create validates and inserts a plugin, assigning ID/timestamps.
func (s *Store) Create(ctx context.Context, p *Plugin) error {
	if err := validate(p); err != nil {
		return err
	}
	p.ID = uuid.New().String()
	now := time.Now().UTC()
	p.CreatedAt, p.UpdatedAt = now, now

	evs, _ := json.Marshal(p.Events)
	env, _ := json.Marshal(p.Env)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO plugins (id, name, enabled, source, events, env, timeout_secs, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, boolToInt(p.Enabled), p.Source, string(evs), string(env),
		p.TimeoutSecs, p.CreatedAt.Format(time.RFC3339Nano), p.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create plugin: %w", err)
	}
	return nil
}

// Update validates and persists changes to an existing plugin.
func (s *Store) Update(ctx context.Context, p *Plugin) error {
	if err := validate(p); err != nil {
		return err
	}
	existing, err := s.Get(ctx, p.ID)
	if err != nil {
		return err
	}
	p.CreatedAt = existing.CreatedAt
	p.UpdatedAt = time.Now().UTC()

	evs, _ := json.Marshal(p.Events)
	env, _ := json.Marshal(p.Env)
	res, err := s.db.ExecContext(ctx, `
		UPDATE plugins SET name = ?, enabled = ?, source = ?, events = ?, env = ?,
		    timeout_secs = ?, updated_at = ?
		WHERE id = ?`,
		p.Name, boolToInt(p.Enabled), p.Source, string(evs), string(env),
		p.TimeoutSecs, p.UpdatedAt.Format(time.RFC3339Nano), p.ID)
	if err != nil {
		return fmt.Errorf("update plugin: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("plugin not found: %s", p.ID)
	}
	return nil
}

// Delete removes a plugin and its run history.
func (s *Store) Delete(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM plugin_runs WHERE plugin_id = ?`, id); err != nil {
		return fmt.Errorf("delete plugin runs: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM plugins WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete plugin: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("plugin not found: %s", id)
	}
	return nil
}

// InsertRun records a run and prunes old history (best-effort).
func (s *Store) InsertRun(ctx context.Context, r *Run) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO plugin_runs (id, plugin_id, plugin_name, topic, success, exit_code, duration_ms, stdout, stderr, error_msg, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.PluginID, r.PluginName, r.Topic, boolToInt(r.Success), r.ExitCode, r.DurationMs,
		r.Stdout, r.Stderr, r.ErrorMsg, r.StartedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}
	s.prune(ctx, r.PluginID)
	return nil
}

// prune enforces per-plugin count and global age retention (best-effort).
func (s *Store) prune(ctx context.Context, pluginID string) {
	// Keep only the most recent maxRunsPerPlugin rows for this plugin.
	_, _ = s.db.ExecContext(ctx, `
		DELETE FROM plugin_runs
		WHERE plugin_id = ? AND id NOT IN (
		    SELECT id FROM plugin_runs WHERE plugin_id = ? ORDER BY started_at DESC LIMIT ?
		)`, pluginID, pluginID, maxRunsPerPlugin)
	cutoff := time.Now().UTC().Add(-maxRunAge).Format(time.RFC3339Nano)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM plugin_runs WHERE started_at < ?`, cutoff)
}

// ListRuns returns recent runs for a plugin, newest first.
func (s *Store) ListRuns(ctx context.Context, pluginID string, limit int) ([]*Run, error) {
	if limit <= 0 || limit > maxRunsPerPlugin {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, plugin_id, plugin_name, topic, success, exit_code, duration_ms, stdout, stderr, error_msg, started_at
		FROM plugin_runs WHERE plugin_id = ? ORDER BY started_at DESC LIMIT ?`, pluginID, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var out []*Run
	for rows.Next() {
		var r Run
		var success int
		var started string
		if err := rows.Scan(&r.ID, &r.PluginID, &r.PluginName, &r.Topic, &success,
			&r.ExitCode, &r.DurationMs, &r.Stdout, &r.Stderr, &r.ErrorMsg, &started); err != nil {
			return nil, err
		}
		r.Success = success != 0
		r.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
		out = append(out, &r)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPlugin(sc scanner) (*Plugin, error) {
	var p Plugin
	var enabled int
	var evs, env, created, updated string
	if err := sc.Scan(&p.ID, &p.Name, &enabled, &p.Source, &evs, &env, &p.TimeoutSecs, &created, &updated); err != nil {
		return nil, err
	}
	p.Enabled = enabled != 0
	_ = json.Unmarshal([]byte(evs), &p.Events)
	_ = json.Unmarshal([]byte(env), &p.Env)
	if p.Env == nil {
		p.Env = map[string]string{}
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &p, nil
}

// validate normalizes and checks a plugin before persistence.
func validate(p *Plugin) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(p.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if len(p.Source) > maxSourceBytes {
		return fmt.Errorf("source exceeds %d bytes", maxSourceBytes)
	}
	if len(p.Events) == 0 {
		return fmt.Errorf("at least one event must be selected")
	}
	for _, e := range p.Events {
		if _, ok := eventByKey(e); !ok {
			return fmt.Errorf("unknown event: %s", e)
		}
	}
	if p.TimeoutSecs <= 0 {
		p.TimeoutSecs = defaultTimeout
	}
	if p.TimeoutSecs > maxTimeoutSecs {
		p.TimeoutSecs = maxTimeoutSecs
	}
	if p.Env == nil {
		p.Env = map[string]string{}
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
