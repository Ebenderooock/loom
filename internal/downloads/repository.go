package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	dbpg "github.com/ebenderooock/loom/internal/storage/db/postgres"
	dbsqlite "github.com/ebenderooock/loom/internal/storage/db/sqlite"
)

// Repository persists Definitions and Health rows. Engine-specific
// adapters (SQLite, Postgres) implement this interface; the rest of
// the package never touches sqlc-generated types directly.
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
}

// --- SQLite adapter -----------------------------------------------

type sqliteRepo struct {
	q *dbsqlite.Queries
}

// NewSQLiteRepository builds a Repository over the sqlc SQLite queries.
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{q: dbsqlite.New(db)}
}

func (s *sqliteRepo) Create(ctx context.Context, def Definition) (Definition, error) {
	cfgJSON, err := encodeConfig(def.Config)
	if err != nil {
		return Definition{}, err
	}
	row, err := s.q.CreateDownloadClient(ctx, dbsqlite.CreateDownloadClientParams{
		ID:              def.ID,
		Name:            def.Name,
		Kind:            string(def.Kind),
		Protocol:        string(def.Protocol),
		Enabled:         boolToInt(def.Enabled),
		Priority:        int64(def.Priority),
		Host:            def.Host,
		Port:            int64(def.Port),
		Tls:             boolToInt(def.TLS),
		Username:        def.Username,
		Password:        def.Password,
		ConfigJson:      string(cfgJSON),
		CategoryDefault: def.CategoryDefault,
		SavePathDefault: def.SavePathDefault,
		RemoveCompleted: boolToInt(def.RemoveCompleted),
		RemoveFailed:    boolToInt(def.RemoveFailed),
	})
	if err != nil {
		return Definition{}, fmt.Errorf("create download client %q: %w", def.ID, err)
	}
	return defFromSQLite(row), nil
}

func (s *sqliteRepo) Get(ctx context.Context, id string) (Definition, error) {
	row, err := s.q.GetDownloadClient(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("get download client %q: %w", id, err)
	}
	return defFromSQLite(row), nil
}

func (s *sqliteRepo) List(ctx context.Context) ([]Definition, error) {
	rows, err := s.q.ListDownloadClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("list download clients: %w", err)
	}
	out := make([]Definition, 0, len(rows))
	for _, r := range rows {
		out = append(out, defFromSQLite(r))
	}
	return out, nil
}

func (s *sqliteRepo) ListEnabled(ctx context.Context) ([]Definition, error) {
	rows, err := s.q.ListEnabledDownloadClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled download clients: %w", err)
	}
	out := make([]Definition, 0, len(rows))
	for _, r := range rows {
		out = append(out, defFromSQLite(r))
	}
	return out, nil
}

func (s *sqliteRepo) Replace(ctx context.Context, def Definition) (Definition, error) {
	cfgJSON, err := encodeConfig(def.Config)
	if err != nil {
		return Definition{}, err
	}
	row, err := s.q.ReplaceDownloadClient(ctx, dbsqlite.ReplaceDownloadClientParams{
		ID:              def.ID,
		Name:            def.Name,
		Kind:            string(def.Kind),
		Protocol:        string(def.Protocol),
		Enabled:         boolToInt(def.Enabled),
		Priority:        int64(def.Priority),
		Host:            def.Host,
		Port:            int64(def.Port),
		Tls:             boolToInt(def.TLS),
		Username:        def.Username,
		Password:        def.Password,
		ConfigJson:      string(cfgJSON),
		CategoryDefault: def.CategoryDefault,
		SavePathDefault: def.SavePathDefault,
		RemoveCompleted: boolToInt(def.RemoveCompleted),
		RemoveFailed:    boolToInt(def.RemoveFailed),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("replace download client %q: %w", def.ID, err)
	}
	return defFromSQLite(row), nil
}

func (s *sqliteRepo) Patch(ctx context.Context, p Patch) (Definition, error) {
	params := dbsqlite.PatchDownloadClientParams{ID: p.ID}
	if p.Name != nil {
		params.Name = sql.NullString{String: *p.Name, Valid: true}
	}
	if p.Enabled != nil {
		params.Enabled = sql.NullInt64{Int64: boolToInt(*p.Enabled), Valid: true}
	}
	if p.Priority != nil {
		params.Priority = sql.NullInt64{Int64: int64(*p.Priority), Valid: true}
	}
	if p.Host != nil {
		params.Host = sql.NullString{String: *p.Host, Valid: true}
	}
	if p.Port != nil {
		params.Port = sql.NullInt64{Int64: int64(*p.Port), Valid: true}
	}
	if p.TLS != nil {
		params.Tls = sql.NullInt64{Int64: boolToInt(*p.TLS), Valid: true}
	}
	if p.Username != nil {
		params.Username = sql.NullString{String: *p.Username, Valid: true}
	}
	if p.Password != nil {
		params.Password = sql.NullString{String: *p.Password, Valid: true}
	}
	if len(p.Config) > 0 {
		params.ConfigJson = sql.NullString{String: string(p.Config), Valid: true}
	}
	if p.CategoryDefault != nil {
		params.CategoryDefault = sql.NullString{String: *p.CategoryDefault, Valid: true}
	}
	if p.SavePathDefault != nil {
		params.SavePathDefault = sql.NullString{String: *p.SavePathDefault, Valid: true}
	}
	if p.RemoveCompleted != nil {
		params.RemoveCompleted = sql.NullInt64{Int64: boolToInt(*p.RemoveCompleted), Valid: true}
	}
	if p.RemoveFailed != nil {
		params.RemoveFailed = sql.NullInt64{Int64: boolToInt(*p.RemoveFailed), Valid: true}
	}
	row, err := s.q.PatchDownloadClient(ctx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("patch download client %q: %w", p.ID, err)
	}
	return defFromSQLite(row), nil
}

func (s *sqliteRepo) Delete(ctx context.Context, id string) error {
	if err := s.q.DeleteDownloadClient(ctx, id); err != nil {
		return fmt.Errorf("delete download client %q: %w", id, err)
	}
	return nil
}

func (s *sqliteRepo) UpsertHealth(ctx context.Context, h Health) error {
	cats, err := encodeCategories(h.LastCategories)
	if err != nil {
		return err
	}
	params := dbsqlite.UpsertDownloadClientHealthParams{
		ClientID:            h.ClientID,
		Status:              string(h.Status),
		LastCheckedAt:       h.LastCheckedAt,
		LastError:           h.LastError,
		ConsecutiveFailures: int64(h.ConsecutiveFailures),
		LastCategoriesJson:  string(cats),
	}
	if h.LastSuccessAt != nil {
		params.LastSuccessAt = sql.NullTime{Time: *h.LastSuccessAt, Valid: true}
	}
	if h.LastFailureAt != nil {
		params.LastFailureAt = sql.NullTime{Time: *h.LastFailureAt, Valid: true}
	}
	if h.LastFreeSpaceBytes != nil {
		params.LastFreeSpaceBytes = sql.NullInt64{Int64: *h.LastFreeSpaceBytes, Valid: true}
	}
	if err := s.q.UpsertDownloadClientHealth(ctx, params); err != nil {
		return fmt.Errorf("upsert download client health %q: %w", h.ClientID, err)
	}
	return nil
}

func (s *sqliteRepo) GetHealth(ctx context.Context, id string) (Health, error) {
	row, err := s.q.GetDownloadClientHealth(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Health{}, ErrNotFound
		}
		return Health{}, fmt.Errorf("get download client health %q: %w", id, err)
	}
	return healthFromSQLite(row)
}

func (s *sqliteRepo) ListHealth(ctx context.Context) (map[string]Health, error) {
	rows, err := s.q.ListDownloadClientHealth(ctx)
	if err != nil {
		return nil, fmt.Errorf("list download client health: %w", err)
	}
	out := make(map[string]Health, len(rows))
	for _, r := range rows {
		h, err := healthFromSQLite(r)
		if err != nil {
			return nil, err
		}
		out[r.ClientID] = h
	}
	return out, nil
}

func defFromSQLite(row dbsqlite.DownloadClient) Definition {
	cfg := json.RawMessage(nil)
	if row.ConfigJson != "" {
		cfg = json.RawMessage(row.ConfigJson)
	}
	return Definition{
		ID:              row.ID,
		Name:            row.Name,
		Kind:            Kind(row.Kind),
		Protocol:        Protocol(row.Protocol),
		Enabled:         row.Enabled != 0,
		Priority:        int(row.Priority),
		Host:            row.Host,
		Port:            int(row.Port),
		TLS:             row.Tls != 0,
		Username:        row.Username,
		Password:        row.Password,
		Config:          cfg,
		CategoryDefault: row.CategoryDefault,
		SavePathDefault: row.SavePathDefault,
		RemoveCompleted: row.RemoveCompleted != 0,
		RemoveFailed:    row.RemoveFailed != 0,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func healthFromSQLite(row dbsqlite.DownloadClientHealth) (Health, error) {
	cats, err := decodeCategories([]byte(row.LastCategoriesJson))
	if err != nil {
		return Health{}, fmt.Errorf("decode categories %q: %w", row.ClientID, err)
	}
	h := Health{
		ClientID:            row.ClientID,
		Status:              HealthStatus(row.Status),
		LastCheckedAt:       row.LastCheckedAt,
		LastError:           row.LastError,
		ConsecutiveFailures: int(row.ConsecutiveFailures),
		LastCategories:      cats,
	}
	if row.LastSuccessAt.Valid {
		t := row.LastSuccessAt.Time
		h.LastSuccessAt = &t
	}
	if row.LastFailureAt.Valid {
		t := row.LastFailureAt.Time
		h.LastFailureAt = &t
	}
	if row.LastFreeSpaceBytes.Valid {
		v := row.LastFreeSpaceBytes.Int64
		h.LastFreeSpaceBytes = &v
	}
	return h, nil
}

// --- Postgres adapter ---------------------------------------------

type pgRepo struct {
	q *dbpg.Queries
}

// NewPostgresRepository builds a Repository over the sqlc Postgres queries.
func NewPostgresRepository(db *sql.DB) Repository {
	return &pgRepo{q: dbpg.New(db)}
}

func (p *pgRepo) Create(ctx context.Context, def Definition) (Definition, error) {
	cfgJSON, err := encodeConfig(def.Config)
	if err != nil {
		return Definition{}, err
	}
	row, err := p.q.CreateDownloadClient(ctx, dbpg.CreateDownloadClientParams{
		ID:              def.ID,
		Name:            def.Name,
		Kind:            string(def.Kind),
		Protocol:        string(def.Protocol),
		Enabled:         def.Enabled,
		Priority:        int32(def.Priority),
		Host:            def.Host,
		Port:            int32(def.Port),
		Tls:             def.TLS,
		Username:        def.Username,
		Password:        def.Password,
		ConfigJson:      cfgJSON,
		CategoryDefault: def.CategoryDefault,
		SavePathDefault: def.SavePathDefault,
		RemoveCompleted: def.RemoveCompleted,
		RemoveFailed:    def.RemoveFailed,
	})
	if err != nil {
		return Definition{}, fmt.Errorf("create download client %q: %w", def.ID, err)
	}
	return defFromPG(row), nil
}

func (p *pgRepo) Get(ctx context.Context, id string) (Definition, error) {
	row, err := p.q.GetDownloadClient(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("get download client %q: %w", id, err)
	}
	return defFromPG(row), nil
}

func (p *pgRepo) List(ctx context.Context) ([]Definition, error) {
	rows, err := p.q.ListDownloadClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("list download clients: %w", err)
	}
	out := make([]Definition, 0, len(rows))
	for _, r := range rows {
		out = append(out, defFromPG(r))
	}
	return out, nil
}

func (p *pgRepo) ListEnabled(ctx context.Context) ([]Definition, error) {
	rows, err := p.q.ListEnabledDownloadClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled download clients: %w", err)
	}
	out := make([]Definition, 0, len(rows))
	for _, r := range rows {
		out = append(out, defFromPG(r))
	}
	return out, nil
}

func (p *pgRepo) Replace(ctx context.Context, def Definition) (Definition, error) {
	cfgJSON, err := encodeConfig(def.Config)
	if err != nil {
		return Definition{}, err
	}
	row, err := p.q.ReplaceDownloadClient(ctx, dbpg.ReplaceDownloadClientParams{
		ID:              def.ID,
		Name:            def.Name,
		Kind:            string(def.Kind),
		Protocol:        string(def.Protocol),
		Enabled:         def.Enabled,
		Priority:        int32(def.Priority),
		Host:            def.Host,
		Port:            int32(def.Port),
		Tls:             def.TLS,
		Username:        def.Username,
		Password:        def.Password,
		ConfigJson:      cfgJSON,
		CategoryDefault: def.CategoryDefault,
		SavePathDefault: def.SavePathDefault,
		RemoveCompleted: def.RemoveCompleted,
		RemoveFailed:    def.RemoveFailed,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("replace download client %q: %w", def.ID, err)
	}
	return defFromPG(row), nil
}

func (p *pgRepo) Patch(ctx context.Context, pp Patch) (Definition, error) {
	params := dbpg.PatchDownloadClientParams{ID: pp.ID}
	if pp.Name != nil {
		params.Name = sql.NullString{String: *pp.Name, Valid: true}
	}
	if pp.Enabled != nil {
		params.Enabled = sql.NullBool{Bool: *pp.Enabled, Valid: true}
	}
	if pp.Priority != nil {
		params.Priority = sql.NullInt32{Int32: int32(*pp.Priority), Valid: true}
	}
	if pp.Host != nil {
		params.Host = sql.NullString{String: *pp.Host, Valid: true}
	}
	if pp.Port != nil {
		params.Port = sql.NullInt32{Int32: int32(*pp.Port), Valid: true}
	}
	if pp.TLS != nil {
		params.Tls = sql.NullBool{Bool: *pp.TLS, Valid: true}
	}
	if pp.Username != nil {
		params.Username = sql.NullString{String: *pp.Username, Valid: true}
	}
	if pp.Password != nil {
		params.Password = sql.NullString{String: *pp.Password, Valid: true}
	}
	if len(pp.Config) > 0 {
		params.ConfigJson = sql.NullString{String: string(pp.Config), Valid: true}
	}
	if pp.CategoryDefault != nil {
		params.CategoryDefault = sql.NullString{String: *pp.CategoryDefault, Valid: true}
	}
	if pp.SavePathDefault != nil {
		params.SavePathDefault = sql.NullString{String: *pp.SavePathDefault, Valid: true}
	}
	if pp.RemoveCompleted != nil {
		params.RemoveCompleted = sql.NullBool{Bool: *pp.RemoveCompleted, Valid: true}
	}
	if pp.RemoveFailed != nil {
		params.RemoveFailed = sql.NullBool{Bool: *pp.RemoveFailed, Valid: true}
	}
	row, err := p.q.PatchDownloadClient(ctx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("patch download client %q: %w", pp.ID, err)
	}
	return defFromPG(row), nil
}

func (p *pgRepo) Delete(ctx context.Context, id string) error {
	if err := p.q.DeleteDownloadClient(ctx, id); err != nil {
		return fmt.Errorf("delete download client %q: %w", id, err)
	}
	return nil
}

func (p *pgRepo) UpsertHealth(ctx context.Context, h Health) error {
	cats, err := encodeCategories(h.LastCategories)
	if err != nil {
		return err
	}
	params := dbpg.UpsertDownloadClientHealthParams{
		ClientID:            h.ClientID,
		Status:              string(h.Status),
		LastCheckedAt:       h.LastCheckedAt,
		LastError:           h.LastError,
		ConsecutiveFailures: int32(h.ConsecutiveFailures),
		LastCategoriesJson:  cats,
	}
	if h.LastSuccessAt != nil {
		params.LastSuccessAt = sql.NullTime{Time: *h.LastSuccessAt, Valid: true}
	}
	if h.LastFailureAt != nil {
		params.LastFailureAt = sql.NullTime{Time: *h.LastFailureAt, Valid: true}
	}
	if h.LastFreeSpaceBytes != nil {
		params.LastFreeSpaceBytes = sql.NullInt64{Int64: *h.LastFreeSpaceBytes, Valid: true}
	}
	if err := p.q.UpsertDownloadClientHealth(ctx, params); err != nil {
		return fmt.Errorf("upsert download client health %q: %w", h.ClientID, err)
	}
	return nil
}

func (p *pgRepo) GetHealth(ctx context.Context, id string) (Health, error) {
	row, err := p.q.GetDownloadClientHealth(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Health{}, ErrNotFound
		}
		return Health{}, fmt.Errorf("get download client health %q: %w", id, err)
	}
	return healthFromPG(row)
}

func (p *pgRepo) ListHealth(ctx context.Context) (map[string]Health, error) {
	rows, err := p.q.ListDownloadClientHealth(ctx)
	if err != nil {
		return nil, fmt.Errorf("list download client health: %w", err)
	}
	out := make(map[string]Health, len(rows))
	for _, r := range rows {
		h, err := healthFromPG(r)
		if err != nil {
			return nil, err
		}
		out[r.ClientID] = h
	}
	return out, nil
}

func defFromPG(row dbpg.DownloadClient) Definition {
	return Definition{
		ID:              row.ID,
		Name:            row.Name,
		Kind:            Kind(row.Kind),
		Protocol:        Protocol(row.Protocol),
		Enabled:         row.Enabled,
		Priority:        int(row.Priority),
		Host:            row.Host,
		Port:            int(row.Port),
		TLS:             row.Tls,
		Username:        row.Username,
		Password:        row.Password,
		Config:          json.RawMessage(row.ConfigJson),
		CategoryDefault: row.CategoryDefault,
		SavePathDefault: row.SavePathDefault,
		RemoveCompleted: row.RemoveCompleted,
		RemoveFailed:    row.RemoveFailed,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func healthFromPG(row dbpg.DownloadClientHealth) (Health, error) {
	cats, err := decodeCategories(row.LastCategoriesJson)
	if err != nil {
		return Health{}, fmt.Errorf("decode categories %q: %w", row.ClientID, err)
	}
	h := Health{
		ClientID:            row.ClientID,
		Status:              HealthStatus(row.Status),
		LastCheckedAt:       row.LastCheckedAt,
		LastError:           row.LastError,
		ConsecutiveFailures: int(row.ConsecutiveFailures),
		LastCategories:      cats,
	}
	if row.LastSuccessAt.Valid {
		t := row.LastSuccessAt.Time
		h.LastSuccessAt = &t
	}
	if row.LastFailureAt.Valid {
		t := row.LastFailureAt.Time
		h.LastFailureAt = &t
	}
	if row.LastFreeSpaceBytes.Valid {
		v := row.LastFreeSpaceBytes.Int64
		h.LastFreeSpaceBytes = &v
	}
	return h, nil
}

// --- Helpers ------------------------------------------------------

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func encodeConfig(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage("{}"), nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("config_json: invalid JSON")
	}
	return raw, nil
}

func encodeCategories(cats []Category) (json.RawMessage, error) {
	if len(cats) == 0 {
		return json.RawMessage("[]"), nil
	}
	return json.Marshal(cats)
}

func decodeCategories(raw []byte) ([]Category, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out []Category
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}


