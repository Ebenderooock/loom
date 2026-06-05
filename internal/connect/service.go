package connect

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Service defines the business logic for the connect module.
type Service interface {
	ListConnections(ctx context.Context) ([]*Connection, error)
	GetConnection(ctx context.Context, id string) (*Connection, error)
	CreateConnection(ctx context.Context, c *Connection) error
	UpdateConnection(ctx context.Context, c *Connection) error
	DeleteConnection(ctx context.Context, id string) error
	TestConnection(ctx context.Context, id string) error
	TestConnectionConfig(ctx context.Context, provider ProviderType, settings ProviderSettings) error
	NotifyAll(ctx context.Context, eventType string) error
}

// service implements Service backed by SQLite.
type service struct {
	db *sql.DB
}

// NewService creates a new connect service.
func NewService(db *sql.DB) Service {
	return &service{db: db}
}

func (s *service) ListConnections(ctx context.Context) ([]*Connection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, provider, enabled, settings, notify_on_import, created_at, updated_at
		FROM connections ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	defer rows.Close()

	var conns []*Connection
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		conns = append(conns, c)
	}
	return conns, rows.Err()
}

func (s *service) GetConnection(ctx context.Context, id string) (*Connection, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, provider, enabled, settings, notify_on_import, created_at, updated_at
		FROM connections WHERE id = ?`, id)

	c, err := scanConnectionRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("connection not found: %s", id)
		}
		return nil, fmt.Errorf("get connection: %w", err)
	}
	return c, nil
}

func (s *service) CreateConnection(ctx context.Context, c *Connection) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now

	settingsJSON, err := json.Marshal(c.Settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO connections
		(id, name, provider, enabled, settings, notify_on_import, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, string(c.Provider), c.Enabled, string(settingsJSON),
		c.NotifyOnImport,
		c.CreatedAt.Format(time.RFC3339), c.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("create connection: %w", err)
	}
	return nil
}

func (s *service) UpdateConnection(ctx context.Context, c *Connection) error {
	c.UpdatedAt = time.Now().UTC()

	settingsJSON, err := json.Marshal(c.Settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE connections
		SET name = ?, provider = ?, enabled = ?, settings = ?,
		    notify_on_import = ?, updated_at = ?
		WHERE id = ?`,
		c.Name, string(c.Provider), c.Enabled, string(settingsJSON),
		c.NotifyOnImport, c.UpdatedAt.Format(time.RFC3339), c.ID)
	if err != nil {
		return fmt.Errorf("update connection: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("connection not found: %s", c.ID)
	}
	return nil
}

func (s *service) DeleteConnection(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM connections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete connection: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("connection not found: %s", id)
	}
	return nil
}

func (s *service) TestConnection(ctx context.Context, id string) error {
	conn, err := s.GetConnection(ctx, id)
	if err != nil {
		return err
	}
	return s.TestConnectionConfig(ctx, conn.Provider, conn.Settings)
}

func (s *service) TestConnectionConfig(ctx context.Context, provider ProviderType, settings ProviderSettings) error {
	p, err := ProviderFor(provider)
	if err != nil {
		return err
	}
	return p.Test(ctx, settings)
}

func (s *service) NotifyAll(ctx context.Context, eventType string) error {
	conns, err := s.ListConnections(ctx)
	if err != nil {
		return fmt.Errorf("list connections for notify: %w", err)
	}

	var targets []*Connection
	for _, c := range conns {
		if c.Enabled && c.NotifyOnImport && eventType == "import" {
			targets = append(targets, c)
		}
	}

	if len(targets) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	for _, conn := range targets {
		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()
			p, err := ProviderFor(c.Provider)
			if err != nil {
				slog.Warn("connect: unknown provider", "provider", c.Provider, "connection", c.Name)
				return
			}
			if err := p.NotifyLibraryUpdate(ctx, c.Settings); err != nil {
				slog.Warn("connect: library update failed", "connection", c.Name, "provider", c.Provider, "error", err)
			}
		}(conn)
	}
	wg.Wait()

	return nil
}

// scanConnection scans a Connection from sql.Rows.
func scanConnection(rows *sql.Rows) (*Connection, error) {
	c := &Connection{}
	var createdAt, updatedAt string
	if err := rows.Scan(&c.ID, &c.Name, &c.Provider, &c.Enabled, &c.Settings,
		&c.NotifyOnImport, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scan connection: %w", err)
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return c, nil
}

// scanConnectionRow scans a Connection from a single sql.Row.
func scanConnectionRow(row *sql.Row) (*Connection, error) {
	c := &Connection{}
	var createdAt, updatedAt string
	if err := row.Scan(&c.ID, &c.Name, &c.Provider, &c.Enabled, &c.Settings,
		&c.NotifyOnImport, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return c, nil
}
