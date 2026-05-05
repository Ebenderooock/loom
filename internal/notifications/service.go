package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Service defines the business logic for the notifications module.
type Service interface {
	ListConnections(ctx context.Context) ([]*Connection, error)
	GetConnection(ctx context.Context, id string) (*Connection, error)
	CreateConnection(ctx context.Context, c *Connection) error
	UpdateConnection(ctx context.Context, c *Connection) error
	DeleteConnection(ctx context.Context, id string) error
	TestConnection(ctx context.Context, id string) error
	Send(ctx context.Context, event EventType, title, message string, data map[string]any) error
	ListHistory(ctx context.Context, limit int) ([]*HistoryEntry, error)
	LogHistory(ctx context.Context, connID *string, eventType, title, message string, success bool, errMsg string) error
}

// service implements Service backed by SQLite.
type service struct {
	db *sql.DB
}

// NewService creates a new notifications service.
func NewService(db *sql.DB) Service {
	return &service{db: db}
}

func (s *service) ListConnections(ctx context.Context) ([]*Connection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, enabled, settings,
		       on_grab, on_download, on_upgrade, on_rename, on_delete,
		       on_health_issue, on_application_update, tags, created_at, updated_at
		FROM notification_connections ORDER BY name ASC`)
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
		SELECT id, name, type, enabled, settings,
		       on_grab, on_download, on_upgrade, on_rename, on_delete,
		       on_health_issue, on_application_update, tags, created_at, updated_at
		FROM notification_connections WHERE id = ?`, id)

	c, err := scanConnectionRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
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
	tagsJSON, err := json.Marshal(c.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO notification_connections
		(id, name, type, enabled, settings, on_grab, on_download, on_upgrade,
		 on_rename, on_delete, on_health_issue, on_application_update, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, string(c.Type), c.Enabled, string(settingsJSON),
		c.OnGrab, c.OnDownload, c.OnUpgrade, c.OnRename, c.OnDelete,
		c.OnHealthIssue, c.OnApplicationUpdate, string(tagsJSON),
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
	tagsJSON, err := json.Marshal(c.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE notification_connections
		SET name = ?, type = ?, enabled = ?, settings = ?,
		    on_grab = ?, on_download = ?, on_upgrade = ?, on_rename = ?,
		    on_delete = ?, on_health_issue = ?, on_application_update = ?,
		    tags = ?, updated_at = ?
		WHERE id = ?`,
		c.Name, string(c.Type), c.Enabled, string(settingsJSON),
		c.OnGrab, c.OnDownload, c.OnUpgrade, c.OnRename,
		c.OnDelete, c.OnHealthIssue, c.OnApplicationUpdate,
		string(tagsJSON), c.UpdatedAt.Format(time.RFC3339), c.ID)
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
	result, err := s.db.ExecContext(ctx, `DELETE FROM notification_connections WHERE id = ?`, id)
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

	sender := senderFor(conn.Type)
	testNotification := Notification{
		EventType: EventOnHealthIssue,
		Title:     "Test Notification",
		Message:   fmt.Sprintf("This is a test notification from connection %q.", conn.Name),
	}

	sendErr := sender.Send(ctx, testNotification, conn.Settings)

	// Log the test to history.
	errMsg := ""
	if sendErr != nil {
		errMsg = sendErr.Error()
	}
	_ = s.logHistory(ctx, &conn.ID, string(testNotification.EventType),
		testNotification.Title, testNotification.Message, sendErr == nil, errMsg)

	return sendErr
}

func (s *service) Send(ctx context.Context, event EventType, title, message string, data map[string]any) error {
	conns, err := s.ListConnections(ctx)
	if err != nil {
		return fmt.Errorf("list connections for send: %w", err)
	}

	// Filter to enabled connections that subscribe to this event.
	var targets []*Connection
	for _, c := range conns {
		if c.Enabled && c.SubscribesTo(event) {
			targets = append(targets, c)
		}
	}

	if len(targets) == 0 {
		return nil
	}

	// Send in parallel so slow webhooks don't block.
	var wg sync.WaitGroup
	for _, conn := range targets {
		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()

			// Render template if the connection has an override.
			msg := message
			if c.Settings.TemplateOverride != "" && data != nil {
				rendered, err := RenderTemplate(c.Settings.TemplateOverride, event, data)
				if err != nil {
					log.Printf("template render failed for %q: %v, using default message", c.Name, err)
				} else {
					msg = rendered
				}
			}

			rendered := Notification{
				EventType: event,
				Title:     title,
				Message:   msg,
				Data:      data,
			}
			sender := senderFor(c.Type)
			sendErr := sender.Send(ctx, rendered, c.Settings)

			errMsg := ""
			if sendErr != nil {
				errMsg = sendErr.Error()
				log.Printf("notification send failed for %q (%s): %v", c.Name, c.Type, sendErr)
			}
			if logErr := s.logHistory(ctx, &c.ID, string(event), title, msg, sendErr == nil, errMsg); logErr != nil {
				log.Printf("failed to log notification history: %v", logErr)
			}
		}(conn)
	}
	wg.Wait()

	return nil
}

func (s *service) ListHistory(ctx context.Context, limit int) ([]*HistoryEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, connection_id, event_type, title, message, success, error_message, sent_at
		FROM notification_history ORDER BY sent_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	defer rows.Close()

	var entries []*HistoryEntry
	for rows.Next() {
		h := &HistoryEntry{}
		var sentAt string
		if err := rows.Scan(&h.ID, &h.ConnectionID, &h.EventType, &h.Title,
			&h.Message, &h.Success, &h.ErrorMessage, &sentAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		h.SentAt, _ = time.Parse(time.RFC3339, sentAt)
		entries = append(entries, h)
	}
	return entries, rows.Err()
}

// LogHistory records a notification send attempt (implements Service).
func (s *service) LogHistory(ctx context.Context, connID *string, eventType, title, message string, success bool, errMsg string) error {
	return s.logHistory(ctx, connID, eventType, title, message, success, errMsg)
}

// logHistory records a notification send attempt.
func (s *service) logHistory(ctx context.Context, connID *string, eventType, title, message string, success bool, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notification_history (connection_id, event_type, title, message, success, error_message, sent_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		connID, eventType, title, message, success, errMsg, time.Now().UTC().Format(time.RFC3339))
	return err
}

// scanConnection scans a Connection from sql.Rows.
func scanConnection(rows *sql.Rows) (*Connection, error) {
	c := &Connection{}
	var createdAt, updatedAt string
	if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.Enabled, &c.Settings,
		&c.OnGrab, &c.OnDownload, &c.OnUpgrade, &c.OnRename, &c.OnDelete,
		&c.OnHealthIssue, &c.OnApplicationUpdate, &c.Tags, &createdAt, &updatedAt); err != nil {
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
	if err := row.Scan(&c.ID, &c.Name, &c.Type, &c.Enabled, &c.Settings,
		&c.OnGrab, &c.OnDownload, &c.OnUpgrade, &c.OnRename, &c.OnDelete,
		&c.OnHealthIssue, &c.OnApplicationUpdate, &c.Tags, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return c, nil
}
