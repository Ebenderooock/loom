package indexers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sqlc-dev/pqtype"

	"github.com/ebenderooock/loom/internal/indexers/throttle"
	dbpg "github.com/ebenderooock/loom/internal/storage/db/postgres"
	dbsqlite "github.com/ebenderooock/loom/internal/storage/db/sqlite"
)

// Repository is the persistence interface for indexer rows and their
// health records. It is engine-neutral; concrete implementations
// dispatch to the matching sqlc-generated package.
type Repository interface {
	Create(ctx context.Context, def Definition) (Definition, error)
	Get(ctx context.Context, id string) (Definition, error)
	List(ctx context.Context) ([]Definition, error)
	ListEnabled(ctx context.Context) ([]Definition, error)
	Replace(ctx context.Context, def Definition) (Definition, error)
	Patch(ctx context.Context, p Patch) (Definition, error)
	Delete(ctx context.Context, id string) error

	UpsertHealth(ctx context.Context, h Health) error
	GetHealth(ctx context.Context, id string) (Health, error)
	ListHealth(ctx context.Context) (map[string]Health, error)

	// GetRateLimit returns the persisted rate-limit config for the
	// indexer. Fields left NULL in the database are returned as zero
	// in the Config; callers should use throttle.Resolve to apply
	// defaults. Returns ErrNotFound if no row matches.
	GetRateLimit(ctx context.Context, id string) (throttle.Config, error)

	// SetRateLimit persists the per-indexer rate-limit dials. Zero or
	// negative values are stored as NULL so the runtime falls back to
	// throttle.Defaults() — pass throttle.Defaults() explicitly to
	// stamp the defaults into the row instead.
	SetRateLimit(ctx context.Context, id string, cfg throttle.Config) error
}

// Patch carries the optional fields acceptable on PATCH /indexers/{id}.
// Nil pointers mean "leave unchanged"; non-nil values are applied.
// ProxyID is a tri-state: nil = unchanged, *""  = clear, *"id" = set.
type Patch struct {
	ID       string
	Name     *string
	Enabled  *bool
	Priority *int
	Tags     *[]string
	ProxyID  *string
}

// --- SQLite adapter -------------------------------------------------

type sqliteRepo struct {
	q *dbsqlite.Queries
}

// NewSQLiteRepository builds a Repository over the sqlc SQLite
// queries. The constructor exists so the storage package can wire it
// without exposing the sqlc package in the public API.
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{q: dbsqlite.New(db)}
}

func (s *sqliteRepo) Create(ctx context.Context, def Definition) (Definition, error) {
	cfg, cats, tags, err := encodeJSONColumns(def)
	if err != nil {
		return Definition{}, err
	}
	row, err := s.q.CreateIndexer(ctx, dbsqlite.CreateIndexerParams{
		ID:             def.ID,
		Kind:           string(def.Kind),
		Name:           def.Name,
		Enabled:        boolToInt(def.Enabled),
		Priority:       int64(def.Priority),
		ConfigJson:     string(cfg),
		CategoriesJson: string(cats),
		TagsJson:       string(tags),
		ProxyID:        nullString(def.ProxyID),
	})
	if err != nil {
		return Definition{}, fmt.Errorf("create indexer %q: %w", def.ID, err)
	}
	return defFromSQLite(row)
}

func (s *sqliteRepo) Get(ctx context.Context, id string) (Definition, error) {
	row, err := s.q.GetIndexer(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("get indexer %q: %w", id, err)
	}
	return defFromSQLite(row)
}

func (s *sqliteRepo) List(ctx context.Context) ([]Definition, error) {
	rows, err := s.q.ListIndexers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list indexers: %w", err)
	}
	return defsFromSQLite(rows)
}

func (s *sqliteRepo) ListEnabled(ctx context.Context) ([]Definition, error) {
	rows, err := s.q.ListEnabledIndexers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled indexers: %w", err)
	}
	return defsFromSQLite(rows)
}

func (s *sqliteRepo) Replace(ctx context.Context, def Definition) (Definition, error) {
	cfg, cats, tags, err := encodeJSONColumns(def)
	if err != nil {
		return Definition{}, err
	}
	row, err := s.q.ReplaceIndexer(ctx, dbsqlite.ReplaceIndexerParams{
		ID:             def.ID,
		Kind:           string(def.Kind),
		Name:           def.Name,
		Enabled:        boolToInt(def.Enabled),
		Priority:       int64(def.Priority),
		ConfigJson:     string(cfg),
		CategoriesJson: string(cats),
		TagsJson:       string(tags),
		ProxyID:        nullString(def.ProxyID),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("replace indexer %q: %w", def.ID, err)
	}
	return defFromSQLite(row)
}

func (s *sqliteRepo) Patch(ctx context.Context, p Patch) (Definition, error) {
	params := dbsqlite.PatchIndexerParams{ID: p.ID}
	if p.Name != nil {
		params.Name = sql.NullString{String: *p.Name, Valid: true}
	}
	if p.Enabled != nil {
		params.Enabled = sql.NullInt64{Int64: boolToInt(*p.Enabled), Valid: true}
	}
	if p.Priority != nil {
		params.Priority = sql.NullInt64{Int64: int64(*p.Priority), Valid: true}
	}
	if p.Tags != nil {
		raw, err := json.Marshal(*p.Tags)
		if err != nil {
			return Definition{}, fmt.Errorf("marshal tags: %w", err)
		}
		params.TagsJson = sql.NullString{String: string(raw), Valid: true}
	}
	row, err := s.q.PatchIndexer(ctx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("patch indexer %q: %w", p.ID, err)
	}
	if p.ProxyID != nil {
		if err := s.q.SetIndexerProxyID(ctx, dbsqlite.SetIndexerProxyIDParams{
			ProxyID: nullString(*p.ProxyID),
			ID:      p.ID,
		}); err != nil {
			return Definition{}, fmt.Errorf("patch indexer proxy %q: %w", p.ID, err)
		}
		// Re-read so the returned Definition reflects the new proxy.
		row, err = s.q.GetIndexer(ctx, p.ID)
		if err != nil {
			return Definition{}, fmt.Errorf("reload indexer %q: %w", p.ID, err)
		}
	}
	return defFromSQLite(row)
}

func (s *sqliteRepo) Delete(ctx context.Context, id string) error {
	if err := s.q.DeleteIndexer(ctx, id); err != nil {
		return fmt.Errorf("delete indexer %q: %w", id, err)
	}
	return nil
}

func (s *sqliteRepo) GetRateLimit(ctx context.Context, id string) (throttle.Config, error) {
	row, err := s.q.GetIndexer(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return throttle.Config{}, ErrNotFound
		}
		return throttle.Config{}, fmt.Errorf("get rate limit %q: %w", id, err)
	}
	return throttle.Config{
		PerMinute:  intFromNullInt64(row.RateLimitPerMin),
		Burst:      intFromNullInt64(row.RateLimitBurst),
		MaxRetries: maxRetriesFromNullInt64(row.RetryMaxAttempts),
	}, nil
}

func (s *sqliteRepo) SetRateLimit(ctx context.Context, id string, cfg throttle.Config) error {
	params := dbsqlite.SetIndexerRateLimitParams{
		ID:               id,
		RateLimitPerMin:  nullInt64FromPositive(cfg.PerMinute),
		RateLimitBurst:   nullInt64FromPositive(cfg.Burst),
		RetryMaxAttempts: nullInt64FromMaxRetries(cfg.MaxRetries),
	}
	if err := s.q.SetIndexerRateLimit(ctx, params); err != nil {
		return fmt.Errorf("set rate limit %q: %w", id, err)
	}
	return nil
}

func (s *sqliteRepo) UpsertHealth(ctx context.Context, h Health) error {
	params := dbsqlite.UpsertIndexerHealthParams{
		IndexerID:     h.IndexerID,
		Status:        string(h.Status),
		LastCheckedAt: h.LastCheckedAt,
		LastError:     h.LastError,
	}
	if h.LastSuccessAt != nil {
		params.LastSuccessAt = sql.NullTime{Time: *h.LastSuccessAt, Valid: true}
	}
	if h.Latency > 0 {
		params.LatencyMs = sql.NullInt64{Int64: h.Latency.Milliseconds(), Valid: true}
	}
	if err := s.q.UpsertIndexerHealth(ctx, params); err != nil {
		return fmt.Errorf("upsert health %q: %w", h.IndexerID, err)
	}
	return nil
}

func (s *sqliteRepo) GetHealth(ctx context.Context, id string) (Health, error) {
	row, err := s.q.GetIndexerHealth(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Health{}, ErrNotFound
		}
		return Health{}, fmt.Errorf("get health %q: %w", id, err)
	}
	return healthFromSQLite(row), nil
}

func (s *sqliteRepo) ListHealth(ctx context.Context) (map[string]Health, error) {
	rows, err := s.q.ListIndexerHealth(ctx)
	if err != nil {
		return nil, fmt.Errorf("list health: %w", err)
	}
	out := make(map[string]Health, len(rows))
	for _, r := range rows {
		out[r.IndexerID] = healthFromSQLite(r)
	}
	return out, nil
}

func defFromSQLite(row dbsqlite.Indexer) (Definition, error) {
	cats, err := decodeCategories([]byte(row.CategoriesJson))
	if err != nil {
		return Definition{}, fmt.Errorf("decode categories %q: %w", row.ID, err)
	}
	tags, err := decodeTags([]byte(row.TagsJson))
	if err != nil {
		return Definition{}, fmt.Errorf("decode tags %q: %w", row.ID, err)
	}
	return Definition{
		ID:         row.ID,
		Kind:       Kind(row.Kind),
		Name:       row.Name,
		Enabled:    row.Enabled != 0,
		Priority:   int(row.Priority),
		Config:     json.RawMessage(row.ConfigJson),
		Categories: cats,
		Tags:       tags,
		ProxyID:    row.ProxyID.String,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}, nil
}

func defsFromSQLite(rows []dbsqlite.Indexer) ([]Definition, error) {
	out := make([]Definition, 0, len(rows))
	for _, r := range rows {
		d, err := defFromSQLite(r)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func healthFromSQLite(row dbsqlite.IndexerHealth) Health {
	h := Health{
		IndexerID:     row.IndexerID,
		Status:        HealthStatus(row.Status),
		LastCheckedAt: row.LastCheckedAt,
		LastError:     row.LastError,
	}
	if row.LastSuccessAt.Valid {
		t := row.LastSuccessAt.Time
		h.LastSuccessAt = &t
	}
	if row.LatencyMs.Valid {
		h.Latency = time.Duration(row.LatencyMs.Int64) * time.Millisecond
		h.LatencyMS = row.LatencyMs.Int64
	}
	return h
}

// --- Postgres adapter -----------------------------------------------

type pgRepo struct {
	q *dbpg.Queries
}

// NewPostgresRepository builds a Repository over the sqlc Postgres
// queries.
func NewPostgresRepository(db *sql.DB) Repository {
	return &pgRepo{q: dbpg.New(db)}
}

func (p *pgRepo) Create(ctx context.Context, def Definition) (Definition, error) {
	cfg, cats, tags, err := encodeJSONColumns(def)
	if err != nil {
		return Definition{}, err
	}
	row, err := p.q.CreateIndexer(ctx, dbpg.CreateIndexerParams{
		ID:             def.ID,
		Kind:           string(def.Kind),
		Name:           def.Name,
		Enabled:        def.Enabled,
		Priority:       int32(def.Priority),
		ConfigJson:     cfg,
		CategoriesJson: cats,
		TagsJson:       tags,
		ProxyID:        nullString(def.ProxyID),
	})
	if err != nil {
		return Definition{}, fmt.Errorf("create indexer %q: %w", def.ID, err)
	}
	return defFromPG(row)
}

func (p *pgRepo) Get(ctx context.Context, id string) (Definition, error) {
	row, err := p.q.GetIndexer(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("get indexer %q: %w", id, err)
	}
	return defFromPG(row)
}

func (p *pgRepo) List(ctx context.Context) ([]Definition, error) {
	rows, err := p.q.ListIndexers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list indexers: %w", err)
	}
	return defsFromPG(rows)
}

func (p *pgRepo) ListEnabled(ctx context.Context) ([]Definition, error) {
	rows, err := p.q.ListEnabledIndexers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled indexers: %w", err)
	}
	return defsFromPG(rows)
}

func (p *pgRepo) Replace(ctx context.Context, def Definition) (Definition, error) {
	cfg, cats, tags, err := encodeJSONColumns(def)
	if err != nil {
		return Definition{}, err
	}
	row, err := p.q.ReplaceIndexer(ctx, dbpg.ReplaceIndexerParams{
		ID:             def.ID,
		Kind:           string(def.Kind),
		Name:           def.Name,
		Enabled:        def.Enabled,
		Priority:       int32(def.Priority),
		ConfigJson:     cfg,
		CategoriesJson: cats,
		TagsJson:       tags,
		ProxyID:        nullString(def.ProxyID),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("replace indexer %q: %w", def.ID, err)
	}
	return defFromPG(row)
}

func (p *pgRepo) Patch(ctx context.Context, pp Patch) (Definition, error) {
	params := dbpg.PatchIndexerParams{ID: pp.ID}
	if pp.Name != nil {
		params.Name = sql.NullString{String: *pp.Name, Valid: true}
	}
	if pp.Enabled != nil {
		params.Enabled = sql.NullBool{Bool: *pp.Enabled, Valid: true}
	}
	if pp.Priority != nil {
		params.Priority = sql.NullInt32{Int32: int32(*pp.Priority), Valid: true}
	}
	if pp.Tags != nil {
		raw, err := json.Marshal(*pp.Tags)
		if err != nil {
			return Definition{}, fmt.Errorf("marshal tags: %w", err)
		}
		params.TagsJson = pqtype.NullRawMessage{RawMessage: raw, Valid: true}
	}
	row, err := p.q.PatchIndexer(ctx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("patch indexer %q: %w", pp.ID, err)
	}
	if pp.ProxyID != nil {
		if err := p.q.SetIndexerProxyID(ctx, dbpg.SetIndexerProxyIDParams{
			ID:      pp.ID,
			ProxyID: nullString(*pp.ProxyID),
		}); err != nil {
			return Definition{}, fmt.Errorf("patch indexer proxy %q: %w", pp.ID, err)
		}
		row, err = p.q.GetIndexer(ctx, pp.ID)
		if err != nil {
			return Definition{}, fmt.Errorf("reload indexer %q: %w", pp.ID, err)
		}
	}
	return defFromPG(row)
}

func (p *pgRepo) Delete(ctx context.Context, id string) error {
	if err := p.q.DeleteIndexer(ctx, id); err != nil {
		return fmt.Errorf("delete indexer %q: %w", id, err)
	}
	return nil
}

func (p *pgRepo) GetRateLimit(ctx context.Context, id string) (throttle.Config, error) {
	row, err := p.q.GetIndexer(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return throttle.Config{}, ErrNotFound
		}
		return throttle.Config{}, fmt.Errorf("get rate limit %q: %w", id, err)
	}
	return throttle.Config{
		PerMinute:  intFromNullInt32(row.RateLimitPerMin),
		Burst:      intFromNullInt32(row.RateLimitBurst),
		MaxRetries: maxRetriesFromNullInt32(row.RetryMaxAttempts),
	}, nil
}

func (p *pgRepo) SetRateLimit(ctx context.Context, id string, cfg throttle.Config) error {
	params := dbpg.SetIndexerRateLimitParams{
		ID:               id,
		RateLimitPerMin:  nullInt32FromPositive(cfg.PerMinute),
		RateLimitBurst:   nullInt32FromPositive(cfg.Burst),
		RetryMaxAttempts: nullInt32FromMaxRetries(cfg.MaxRetries),
	}
	if err := p.q.SetIndexerRateLimit(ctx, params); err != nil {
		return fmt.Errorf("set rate limit %q: %w", id, err)
	}
	return nil
}

func (p *pgRepo) UpsertHealth(ctx context.Context, h Health) error {
	params := dbpg.UpsertIndexerHealthParams{
		IndexerID:     h.IndexerID,
		Status:        string(h.Status),
		LastCheckedAt: h.LastCheckedAt,
		LastError:     h.LastError,
	}
	if h.LastSuccessAt != nil {
		params.LastSuccessAt = sql.NullTime{Time: *h.LastSuccessAt, Valid: true}
	}
	if h.Latency > 0 {
		params.LatencyMs = sql.NullInt32{Int32: int32(h.Latency.Milliseconds()), Valid: true}
	}
	if err := p.q.UpsertIndexerHealth(ctx, params); err != nil {
		return fmt.Errorf("upsert health %q: %w", h.IndexerID, err)
	}
	return nil
}

func (p *pgRepo) GetHealth(ctx context.Context, id string) (Health, error) {
	row, err := p.q.GetIndexerHealth(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Health{}, ErrNotFound
		}
		return Health{}, fmt.Errorf("get health %q: %w", id, err)
	}
	return healthFromPG(row), nil
}

func (p *pgRepo) ListHealth(ctx context.Context) (map[string]Health, error) {
	rows, err := p.q.ListIndexerHealth(ctx)
	if err != nil {
		return nil, fmt.Errorf("list health: %w", err)
	}
	out := make(map[string]Health, len(rows))
	for _, r := range rows {
		out[r.IndexerID] = healthFromPG(r)
	}
	return out, nil
}

func defFromPG(row dbpg.Indexer) (Definition, error) {
	cats, err := decodeCategories(row.CategoriesJson)
	if err != nil {
		return Definition{}, fmt.Errorf("decode categories %q: %w", row.ID, err)
	}
	tags, err := decodeTags(row.TagsJson)
	if err != nil {
		return Definition{}, fmt.Errorf("decode tags %q: %w", row.ID, err)
	}
	return Definition{
		ID:         row.ID,
		Kind:       Kind(row.Kind),
		Name:       row.Name,
		Enabled:    row.Enabled,
		Priority:   int(row.Priority),
		Config:     row.ConfigJson,
		Categories: cats,
		Tags:       tags,
		ProxyID:    row.ProxyID.String,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}, nil
}

func defsFromPG(rows []dbpg.Indexer) ([]Definition, error) {
	out := make([]Definition, 0, len(rows))
	for _, r := range rows {
		d, err := defFromPG(r)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func healthFromPG(row dbpg.IndexerHealth) Health {
	h := Health{
		IndexerID:     row.IndexerID,
		Status:        HealthStatus(row.Status),
		LastCheckedAt: row.LastCheckedAt,
		LastError:     row.LastError,
	}
	if row.LastSuccessAt.Valid {
		t := row.LastSuccessAt.Time
		h.LastSuccessAt = &t
	}
	if row.LatencyMs.Valid {
		h.Latency = time.Duration(row.LatencyMs.Int32) * time.Millisecond
		h.LatencyMS = int64(row.LatencyMs.Int32)
	}
	return h
}

// --- Encoding helpers ----------------------------------------------

func encodeJSONColumns(def Definition) (cfg, cats, tags json.RawMessage, err error) {
	cfg = def.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	if !json.Valid(cfg) {
		return nil, nil, nil, fmt.Errorf("indexer %q: config is not valid JSON", def.ID)
	}
	categories := def.Categories
	if categories == nil {
		categories = []Category{}
	}
	cats, err = json.Marshal(categories)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal categories: %w", err)
	}
	tagList := def.Tags
	if tagList == nil {
		tagList = []string{}
	}
	tags, err = json.Marshal(tagList)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal tags: %w", err)
	}
	return cfg, cats, tags, nil
}

func decodeCategories(raw []byte) ([]Category, error) {
	if len(raw) == 0 {
		return []Category{}, nil
	}
	var out []Category
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Category{}
	}
	return out, nil
}

func decodeTags(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// nullString lifts an empty string to NULL for the wire-format. Both
// engines model `proxy_id` as nullable; the repository keeps the
// public API stringy and translates here.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// --- Rate-limit nullable-integer helpers -----------------------------
//
// PerMinute and Burst use the convention "<= 0 means use default", so
// we collapse those to NULL. MaxRetries uses "< 0 means use default,
// 0 means never retry" — we have to preserve the explicit 0 by storing
// it as 0 and only NULLing when the caller asked for the default.

func nullInt64FromPositive(v int) sql.NullInt64 {
	if v <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(v), Valid: true}
}

func nullInt64FromMaxRetries(v int) sql.NullInt64 {
	if v < 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(v), Valid: true}
}

func intFromNullInt64(v sql.NullInt64) int {
	if !v.Valid {
		return 0
	}
	return int(v.Int64)
}

func maxRetriesFromNullInt64(v sql.NullInt64) int {
	if !v.Valid {
		return -1 // sentinel for "use default"
	}
	return int(v.Int64)
}

func nullInt32FromPositive(v int) sql.NullInt32 {
	if v <= 0 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(v), Valid: true}
}

func nullInt32FromMaxRetries(v int) sql.NullInt32 {
	if v < 0 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(v), Valid: true}
}

func intFromNullInt32(v sql.NullInt32) int {
	if !v.Valid {
		return 0
	}
	return int(v.Int32)
}

func maxRetriesFromNullInt32(v sql.NullInt32) int {
	if !v.Valid {
		return -1
	}
	return int(v.Int32)
}
