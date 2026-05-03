package proxies

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	dbpg "github.com/loomctl/loom/internal/storage/db/postgres"
	dbsqlite "github.com/loomctl/loom/internal/storage/db/sqlite"
	"github.com/sqlc-dev/pqtype"
)

// Repository is the persistence interface for proxy rows. It is
// engine-neutral; concrete implementations dispatch to the matching
// sqlc-generated package, mirroring internal/indexers/repository.go.
type Repository interface {
	Create(ctx context.Context, p Proxy) (Proxy, error)
	Get(ctx context.Context, id string) (Proxy, error)
	List(ctx context.Context) ([]Proxy, error)
	Replace(ctx context.Context, p Proxy) (Proxy, error)
	Patch(ctx context.Context, p Patch) (Proxy, error)
	Delete(ctx context.Context, id string) error
	IndexerIDsUsing(ctx context.Context, id string) ([]string, error)
}

// --- SQLite adapter -------------------------------------------------

type sqliteRepo struct{ q *dbsqlite.Queries }

// NewSQLiteRepository returns a Repository over the SQLite sqlc
// queries.
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{q: dbsqlite.New(db)}
}

func (s *sqliteRepo) Create(ctx context.Context, p Proxy) (Proxy, error) {
	cfg := configBytes(p.Config)
	row, err := s.q.CreateProxy(ctx, dbsqlite.CreateProxyParams{
		ID:         p.ID,
		Kind:       string(p.Kind),
		Name:       p.Name,
		Enabled:    boolToInt(p.Enabled),
		ConfigJson: string(cfg),
	})
	if err != nil {
		return Proxy{}, fmt.Errorf("create proxy %q: %w", p.ID, err)
	}
	return proxyFromSQLite(row), nil
}

func (s *sqliteRepo) Get(ctx context.Context, id string) (Proxy, error) {
	row, err := s.q.GetProxy(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Proxy{}, ErrNotFound
		}
		return Proxy{}, fmt.Errorf("get proxy %q: %w", id, err)
	}
	return proxyFromSQLite(row), nil
}

func (s *sqliteRepo) List(ctx context.Context) ([]Proxy, error) {
	rows, err := s.q.ListProxies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list proxies: %w", err)
	}
	out := make([]Proxy, 0, len(rows))
	for _, r := range rows {
		out = append(out, proxyFromSQLite(r))
	}
	return out, nil
}

func (s *sqliteRepo) Replace(ctx context.Context, p Proxy) (Proxy, error) {
	cfg := configBytes(p.Config)
	row, err := s.q.ReplaceProxy(ctx, dbsqlite.ReplaceProxyParams{
		ID:         p.ID,
		Kind:       string(p.Kind),
		Name:       p.Name,
		Enabled:    boolToInt(p.Enabled),
		ConfigJson: string(cfg),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Proxy{}, ErrNotFound
		}
		return Proxy{}, fmt.Errorf("replace proxy %q: %w", p.ID, err)
	}
	return proxyFromSQLite(row), nil
}

func (s *sqliteRepo) Patch(ctx context.Context, p Patch) (Proxy, error) {
	params := dbsqlite.PatchProxyParams{ID: p.ID}
	if p.Kind != nil {
		params.Kind = sql.NullString{String: string(*p.Kind), Valid: true}
	}
	if p.Name != nil {
		params.Name = sql.NullString{String: *p.Name, Valid: true}
	}
	if p.Enabled != nil {
		params.Enabled = sql.NullInt64{Int64: boolToInt(*p.Enabled), Valid: true}
	}
	if p.Config != nil {
		params.ConfigJson = sql.NullString{String: string(configBytes(*p.Config)), Valid: true}
	}
	row, err := s.q.PatchProxy(ctx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Proxy{}, ErrNotFound
		}
		return Proxy{}, fmt.Errorf("patch proxy %q: %w", p.ID, err)
	}
	return proxyFromSQLite(row), nil
}

func (s *sqliteRepo) Delete(ctx context.Context, id string) error {
	if err := s.q.DeleteProxy(ctx, id); err != nil {
		return fmt.Errorf("delete proxy %q: %w", id, err)
	}
	return nil
}

func (s *sqliteRepo) IndexerIDsUsing(ctx context.Context, id string) ([]string, error) {
	rows, err := s.q.ListIndexerIDsByProxyID(ctx, sql.NullString{String: id, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list indexers using proxy %q: %w", id, err)
	}
	if rows == nil {
		rows = []string{}
	}
	return rows, nil
}

func proxyFromSQLite(row dbsqlite.Proxy) Proxy {
	return Proxy{
		ID:        row.ID,
		Kind:      Kind(row.Kind),
		Name:      row.Name,
		Enabled:   row.Enabled != 0,
		Config:    json.RawMessage(row.ConfigJson),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// --- Postgres adapter -----------------------------------------------

type pgRepo struct{ q *dbpg.Queries }

// NewPostgresRepository returns a Repository over the Postgres sqlc
// queries.
func NewPostgresRepository(db *sql.DB) Repository {
	return &pgRepo{q: dbpg.New(db)}
}

func (p *pgRepo) Create(ctx context.Context, x Proxy) (Proxy, error) {
	cfg := configBytes(x.Config)
	row, err := p.q.CreateProxy(ctx, dbpg.CreateProxyParams{
		ID:         x.ID,
		Kind:       string(x.Kind),
		Name:       x.Name,
		Enabled:    x.Enabled,
		ConfigJson: cfg,
	})
	if err != nil {
		return Proxy{}, fmt.Errorf("create proxy %q: %w", x.ID, err)
	}
	return proxyFromPG(row), nil
}

func (p *pgRepo) Get(ctx context.Context, id string) (Proxy, error) {
	row, err := p.q.GetProxy(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Proxy{}, ErrNotFound
		}
		return Proxy{}, fmt.Errorf("get proxy %q: %w", id, err)
	}
	return proxyFromPG(row), nil
}

func (p *pgRepo) List(ctx context.Context) ([]Proxy, error) {
	rows, err := p.q.ListProxies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list proxies: %w", err)
	}
	out := make([]Proxy, 0, len(rows))
	for _, r := range rows {
		out = append(out, proxyFromPG(r))
	}
	return out, nil
}

func (p *pgRepo) Replace(ctx context.Context, x Proxy) (Proxy, error) {
	cfg := configBytes(x.Config)
	row, err := p.q.ReplaceProxy(ctx, dbpg.ReplaceProxyParams{
		ID:         x.ID,
		Kind:       string(x.Kind),
		Name:       x.Name,
		Enabled:    x.Enabled,
		ConfigJson: cfg,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Proxy{}, ErrNotFound
		}
		return Proxy{}, fmt.Errorf("replace proxy %q: %w", x.ID, err)
	}
	return proxyFromPG(row), nil
}

func (p *pgRepo) Patch(ctx context.Context, pp Patch) (Proxy, error) {
	params := dbpg.PatchProxyParams{ID: pp.ID}
	if pp.Kind != nil {
		params.Kind = sql.NullString{String: string(*pp.Kind), Valid: true}
	}
	if pp.Name != nil {
		params.Name = sql.NullString{String: *pp.Name, Valid: true}
	}
	if pp.Enabled != nil {
		params.Enabled = sql.NullBool{Bool: *pp.Enabled, Valid: true}
	}
	if pp.Config != nil {
		params.ConfigJson = pqtype.NullRawMessage{RawMessage: configBytes(*pp.Config), Valid: true}
	}
	row, err := p.q.PatchProxy(ctx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Proxy{}, ErrNotFound
		}
		return Proxy{}, fmt.Errorf("patch proxy %q: %w", pp.ID, err)
	}
	return proxyFromPG(row), nil
}

func (p *pgRepo) Delete(ctx context.Context, id string) error {
	if err := p.q.DeleteProxy(ctx, id); err != nil {
		return fmt.Errorf("delete proxy %q: %w", id, err)
	}
	return nil
}

func (p *pgRepo) IndexerIDsUsing(ctx context.Context, id string) ([]string, error) {
	rows, err := p.q.ListIndexerIDsByProxyID(ctx, sql.NullString{String: id, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list indexers using proxy %q: %w", id, err)
	}
	if rows == nil {
		rows = []string{}
	}
	return rows, nil
}

func proxyFromPG(row dbpg.Proxy) Proxy {
	return Proxy{
		ID:        row.ID,
		Kind:      Kind(row.Kind),
		Name:      row.Name,
		Enabled:   row.Enabled,
		Config:    row.ConfigJson,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// --- helpers --------------------------------------------------------

func configBytes(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
