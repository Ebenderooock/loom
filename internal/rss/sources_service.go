package rss

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/loomctl/loom/internal/storage"
	dbpg "github.com/loomctl/loom/internal/storage/db/postgres"
	dbsqlite "github.com/loomctl/loom/internal/storage/db/sqlite"
)

// SourceType identifies the kind of RSS source: RSS feed or web scraper.
type SourceType string

const (
	SourceTypeRSS     SourceType = "rss"
	SourceTypeScraper SourceType = "scraper"
)

// RSSSourceConfig holds the configuration for RSS feed sources.
type RSSSourceConfig struct {
	URL      string `json:"url"`
	AuthType string `json:"auth_type,omitempty"` // 'none', 'basic', 'apikey'
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
}

// ScraperConfig holds the configuration for web scraper sources.
type ScraperConfig struct {
	URL              string `json:"url"`
	AuthType         string `json:"auth_type,omitempty"` // 'none', 'basic', 'apikey'
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	APIKey           string `json:"api_key,omitempty"`
	SelectorType     string `json:"selector_type"` // 'css' or 'xpath'
	ItemSelector     string `json:"item_selector"`
	TitleSelector    string `json:"title_selector"`
	LinkSelector     string `json:"link_selector,omitempty"`
	PublishedSelector string `json:"published_selector,omitempty"`
	Pagination       struct {
		Type       string `json:"type"` // 'none', 'page_number', 'offset'
		PageParam  string `json:"page_param,omitempty"`  // e.g. 'page' or 'p'
		OffsetParam string `json:"offset_param,omitempty"` // e.g. 'offset'
		PageSize   int    `json:"page_size,omitempty"`
	} `json:"pagination,omitempty"`
}

// UserSource represents a user-configured RSS/scraper source.
type UserSource struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        SourceType `json:"type"`
	Enabled     bool      `json:"enabled"`
	Config      json.RawMessage `json:"config"`
	LastSyncAt  *string   `json:"last_sync_at,omitempty"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

// SourcesService manages user-configured RSS and scraper sources.
type SourcesService struct {
	logger *slog.Logger
	db     storage.DB
}

// NewSourcesService constructs a SourcesService.
func NewSourcesService(logger *slog.Logger, db storage.DB) *SourcesService {
	if logger == nil {
		logger = slog.Default()
	}
	return &SourcesService{
		logger: logger,
		db:     db,
	}
}

// CreateSource creates a new user-configured source.
func (s *SourcesService) CreateSource(ctx context.Context, id, name string, typ SourceType, config json.RawMessage) (*UserSource, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("source name is required")
	}
	if typ != SourceTypeRSS && typ != SourceTypeScraper {
		return nil, fmt.Errorf("invalid source type: %s", typ)
	}

	sqlDB := s.db.DB()

	switch s.db.Engine() {
	case storage.EngineSQLite:
		q := dbsqlite.New(sqlDB)
		row, err := q.CreateUserSource(ctx, dbsqlite.CreateUserSourceParams{
			ID:      id,
			Name:    name,
			Type:    string(typ),
			Enabled: sql.NullBool{Bool: true, Valid: true},
			Config:  string(config),
		})
		if err != nil {
			return nil, fmt.Errorf("create source: %w", err)
		}
		return rowToUserSource(row), nil

	case storage.EnginePostgres:
		q := dbpg.New(sqlDB)
		row, err := q.CreateUserSource(ctx, dbpg.CreateUserSourceParams{
			ID:      id,
			Name:    name,
			Type:    string(typ),
			Enabled: sql.NullBool{Bool: true, Valid: true},
			Config:  string(config),
		})
		if err != nil {
			return nil, fmt.Errorf("create source: %w", err)
		}
		return rowToUserSource(row), nil

	default:
		return nil, fmt.Errorf("unsupported storage engine: %v", s.db.Engine())
	}
}

// GetSource retrieves a source by ID.
func (s *SourcesService) GetSource(ctx context.Context, id string) (*UserSource, error) {
	sqlDB := s.db.DB()

	switch s.db.Engine() {
	case storage.EngineSQLite:
		q := dbsqlite.New(sqlDB)
		row, err := q.GetUserSource(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("get source: %w", err)
		}
		return rowToUserSource(row), nil

	case storage.EnginePostgres:
		q := dbpg.New(sqlDB)
		row, err := q.GetUserSource(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("get source: %w", err)
		}
		return rowToUserSource(row), nil

	default:
		return nil, fmt.Errorf("unsupported storage engine: %v", s.db.Engine())
	}
}

// GetSourceByName retrieves a source by name.
func (s *SourcesService) GetSourceByName(ctx context.Context, name string) (*UserSource, error) {
	sqlDB := s.db.DB()

	switch s.db.Engine() {
	case storage.EngineSQLite:
		q := dbsqlite.New(sqlDB)
		row, err := q.GetUserSourceByName(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("get source: %w", err)
		}
		return rowToUserSource(row), nil

	case storage.EnginePostgres:
		q := dbpg.New(sqlDB)
		row, err := q.GetUserSourceByName(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("get source: %w", err)
		}
		return rowToUserSource(row), nil

	default:
		return nil, fmt.Errorf("unsupported storage engine: %v", s.db.Engine())
	}
}

// ListSources retrieves all sources.
func (s *SourcesService) ListSources(ctx context.Context) ([]*UserSource, error) {
	sqlDB := s.db.DB()

	switch s.db.Engine() {
	case storage.EngineSQLite:
		q := dbsqlite.New(sqlDB)
		rows, err := q.ListUserSources(ctx)
		if err != nil {
			return nil, fmt.Errorf("list sources: %w", err)
		}
		var sources []*UserSource
		for _, row := range rows {
			sources = append(sources, rowToUserSource(row))
		}
		return sources, nil

	case storage.EnginePostgres:
		q := dbpg.New(sqlDB)
		rows, err := q.ListUserSources(ctx)
		if err != nil {
			return nil, fmt.Errorf("list sources: %w", err)
		}
		var sources []*UserSource
		for _, row := range rows {
			sources = append(sources, rowToUserSource(row))
		}
		return sources, nil

	default:
		return nil, fmt.Errorf("unsupported storage engine: %v", s.db.Engine())
	}
}

// ListEnabledSources retrieves all enabled sources.
func (s *SourcesService) ListEnabledSources(ctx context.Context) ([]*UserSource, error) {
	sqlDB := s.db.DB()

	switch s.db.Engine() {
	case storage.EngineSQLite:
		q := dbsqlite.New(sqlDB)
		rows, err := q.ListEnabledUserSources(ctx)
		if err != nil {
			return nil, fmt.Errorf("list enabled sources: %w", err)
		}
		var sources []*UserSource
		for _, row := range rows {
			sources = append(sources, rowToUserSource(row))
		}
		return sources, nil

	case storage.EnginePostgres:
		q := dbpg.New(sqlDB)
		rows, err := q.ListEnabledUserSources(ctx)
		if err != nil {
			return nil, fmt.Errorf("list enabled sources: %w", err)
		}
		var sources []*UserSource
		for _, row := range rows {
			sources = append(sources, rowToUserSource(row))
		}
		return sources, nil

	default:
		return nil, fmt.Errorf("unsupported storage engine: %v", s.db.Engine())
	}
}

// ListSourcesByType retrieves all sources of a given type.
func (s *SourcesService) ListSourcesByType(ctx context.Context, typ SourceType) ([]*UserSource, error) {
	sqlDB := s.db.DB()

	switch s.db.Engine() {
	case storage.EngineSQLite:
		q := dbsqlite.New(sqlDB)
		rows, err := q.ListUserSourcesByType(ctx, string(typ))
		if err != nil {
			return nil, fmt.Errorf("list sources by type: %w", err)
		}
		var sources []*UserSource
		for _, row := range rows {
			sources = append(sources, rowToUserSource(row))
		}
		return sources, nil

	case storage.EnginePostgres:
		q := dbpg.New(sqlDB)
		rows, err := q.ListUserSourcesByType(ctx, string(typ))
		if err != nil {
			return nil, fmt.Errorf("list sources by type: %w", err)
		}
		var sources []*UserSource
		for _, row := range rows {
			sources = append(sources, rowToUserSource(row))
		}
		return sources, nil

	default:
		return nil, fmt.Errorf("unsupported storage engine: %v", s.db.Engine())
	}
}

// UpdateSource updates an existing source.
func (s *SourcesService) UpdateSource(ctx context.Context, id, name string, typ SourceType, enabled bool, config json.RawMessage) (*UserSource, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("source name is required")
	}
	if typ != SourceTypeRSS && typ != SourceTypeScraper {
		return nil, fmt.Errorf("invalid source type: %s", typ)
	}

	sqlDB := s.db.DB()

	switch s.db.Engine() {
	case storage.EngineSQLite:
		q := dbsqlite.New(sqlDB)
		row, err := q.UpdateUserSource(ctx, dbsqlite.UpdateUserSourceParams{
			ID:      id,
			Name:    name,
			Type:    string(typ),
			Enabled: sql.NullBool{Bool: enabled, Valid: true},
			Config:  string(config),
		})
		if err != nil {
			return nil, fmt.Errorf("update source: %w", err)
		}
		return rowToUserSource(row), nil

	case storage.EnginePostgres:
		q := dbpg.New(sqlDB)
		row, err := q.UpdateUserSource(ctx, dbpg.UpdateUserSourceParams{
			ID:      id,
			Name:    name,
			Type:    string(typ),
			Enabled: sql.NullBool{Bool: enabled, Valid: true},
			Config:  string(config),
		})
		if err != nil {
			return nil, fmt.Errorf("update source: %w", err)
		}
		return rowToUserSource(row), nil

	default:
		return nil, fmt.Errorf("unsupported storage engine: %v", s.db.Engine())
	}
}

// DeleteSource deletes a source by ID.
func (s *SourcesService) DeleteSource(ctx context.Context, id string) error {
	sqlDB := s.db.DB()

	switch s.db.Engine() {
	case storage.EngineSQLite:
		q := dbsqlite.New(sqlDB)
		if err := q.DeleteUserSource(ctx, id); err != nil {
			return fmt.Errorf("delete source: %w", err)
		}
		return nil

	case storage.EnginePostgres:
		q := dbpg.New(sqlDB)
		if err := q.DeleteUserSource(ctx, id); err != nil {
			return fmt.Errorf("delete source: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("unsupported storage engine: %v", s.db.Engine())
	}
}

// Convert row interface to UserSource.
func rowToUserSource(row interface{}) *UserSource {
	switch r := row.(type) {
	case dbsqlite.UserSource:
		us := &UserSource{
			ID:      r.ID,
			Name:    r.Name,
			Type:    SourceType(r.Type),
			Enabled: r.Enabled.Bool,
			Config:  []byte(r.Config),
		}
		if r.CreatedAt.Valid {
			us.CreatedAt = r.CreatedAt.Time.String()
		}
		if r.UpdatedAt.Valid {
			us.UpdatedAt = r.UpdatedAt.Time.String()
		}
		if r.LastSyncAt.Valid {
			lastSyncAt := r.LastSyncAt.Time.String()
			us.LastSyncAt = &lastSyncAt
		}
		return us

	case dbpg.UserSource:
		us := &UserSource{
			ID:      r.ID,
			Name:    r.Name,
			Type:    SourceType(r.Type),
			Enabled: r.Enabled.Bool,
			Config:  []byte(r.Config),
		}
		if r.CreatedAt.Valid {
			us.CreatedAt = r.CreatedAt.Time.String()
		}
		if r.UpdatedAt.Valid {
			us.UpdatedAt = r.UpdatedAt.Time.String()
		}
		if r.LastSyncAt.Valid {
			lastSyncAt := r.LastSyncAt.Time.String()
			us.LastSyncAt = &lastSyncAt
		}
		return us

	default:
		return nil
	}
}

// MakeFeedSource creates a FeedSource from a UserSource config.
// Returns the appropriate source type (RSS or Scraper) or an error if config is invalid.
func (s *SourcesService) MakeFeedSource(us *UserSource) (FeedSource, error) {
	if us == nil {
		return nil, fmt.Errorf("user source is nil")
	}

	switch us.Type {
	case SourceTypeRSS:
		var cfg RSSSourceConfig
		if err := json.Unmarshal(us.Config, &cfg); err != nil {
			return nil, fmt.Errorf("invalid RSS config: %w", err)
		}
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, fmt.Errorf("RSS URL is required")
		}
		return NewGenericRSSFeedSource(us.ID, us.Name, cfg.URL, time.Hour, s.logger), nil

	case SourceTypeScraper:
		var cfg ScraperConfig
		if err := json.Unmarshal(us.Config, &cfg); err != nil {
			return nil, fmt.Errorf("invalid scraper config: %w", err)
		}
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, fmt.Errorf("scraper URL is required")
		}
		if strings.TrimSpace(cfg.ItemSelector) == "" {
			return nil, fmt.Errorf("item selector is required")
		}
		if strings.TrimSpace(cfg.TitleSelector) == "" {
			return nil, fmt.Errorf("title selector is required")
		}
		return NewWebScraper(s.logger, us.ID, us.Name, cfg)

	default:
		return nil, fmt.Errorf("unknown source type: %s", us.Type)
	}
}
