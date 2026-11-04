package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
)

// ArtworkSettings holds the optional API keys for the music artwork sources.
// The keys are write-only: the API only reports whether each one is set.
type ArtworkSettings struct {
	FanartAPIKey  string `json:"-"`
	AudioDBAPIKey string `json:"-"`
	FanartKeySet  bool   `json:"fanart_api_key_set"`
	AudioDBKeySet bool   `json:"audiodb_api_key_set"`
	// FanartKeyFromEnv / AudioDBKeyFromEnv report a server environment key
	// that is used while no key is stored.
	FanartKeyFromEnv  bool `json:"fanart_api_key_from_env"`
	AudioDBKeyFromEnv bool `json:"audiodb_api_key_from_env"`
}

// ArtworkSettingsUpdate is the request body for saving artwork keys. An empty
// key keeps the stored value unless the matching Clear flag is set.
type ArtworkSettingsUpdate struct {
	FanartAPIKey       string `json:"fanart_api_key"`
	AudioDBAPIKey      string `json:"audiodb_api_key"`
	ClearFanartAPIKey  bool   `json:"clear_fanart_api_key"`
	ClearAudioDBAPIKey bool   `json:"clear_audiodb_api_key"`
}

// ArtworkTestResult reports whether each artwork source accepted its key.
type ArtworkTestResult struct {
	OK            bool   `json:"ok"`
	FanartTested  bool   `json:"fanart_tested"`
	FanartOK      bool   `json:"fanart_ok"`
	FanartError   string `json:"fanart_error,omitempty"`
	AudioDBOK     bool   `json:"audiodb_ok"`
	AudioDBError  string `json:"audiodb_error,omitempty"`
	AudioDBShared bool   `json:"audiodb_shared_key"`
}

// ArtworkKeyApplier applies keys to the live artwork provider.
type ArtworkKeyApplier interface {
	SetKeys(fanartKey, audioDBKey string)
	TestKeys(ctx context.Context, fanartKey, audioDBKey string) (fanartErr, audioDBErr error)
}

// ArtworkSettingsStore persists the single artwork settings row.
type ArtworkSettingsStore interface {
	Get(ctx context.Context) (ArtworkSettings, error)
	Save(ctx context.Context, s ArtworkSettings) error
}

// NewArtworkSettingsStore returns a SQL-backed store.
func NewArtworkSettingsStore(db *sql.DB, engine ...string) ArtworkSettingsStore {
	postgres := len(engine) > 0 && engine[0] == "postgres"
	return &sqlArtworkSettingsStore{db: &musicDB{raw: db, postgres: postgres}}
}

type sqlArtworkSettingsStore struct {
	db *musicDB
}

func (s *sqlArtworkSettingsStore) Get(ctx context.Context) (ArtworkSettings, error) {
	var out ArtworkSettings
	err := s.db.QueryRowContext(ctx, `SELECT fanart_api_key, audiodb_api_key
		FROM music_artwork_settings WHERE id = 1`).Scan(&out.FanartAPIKey, &out.AudioDBAPIKey)
	if errors.Is(err, sql.ErrNoRows) {
		return ArtworkSettings{}, nil
	}
	if err != nil {
		return ArtworkSettings{}, err
	}
	return out, nil
}

func (s *sqlArtworkSettingsStore) Save(ctx context.Context, a ArtworkSettings) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO music_artwork_settings (id, fanart_api_key, audiodb_api_key)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET fanart_api_key = excluded.fanart_api_key,
			audiodb_api_key = excluded.audiodb_api_key`,
		a.FanartAPIKey, a.AudioDBAPIKey)
	return err
}

// ArtworkSettingsService stores artwork keys and applies them. Stored keys
// win; the environment keys are used when nothing is stored.
type ArtworkSettingsService struct {
	mu         sync.Mutex
	store      ArtworkSettingsStore
	applier    ArtworkKeyApplier
	envFanart  string
	envAudioDB string
}

// NewArtworkSettingsService builds the service. envFanart and envAudioDB are
// optional server-level fallback keys.
func NewArtworkSettingsService(store ArtworkSettingsStore, applier ArtworkKeyApplier, envFanart, envAudioDB string) *ArtworkSettingsService {
	return &ArtworkSettingsService{
		store:      store,
		applier:    applier,
		envFanart:  strings.TrimSpace(envFanart),
		envAudioDB: strings.TrimSpace(envAudioDB),
	}
}

// Get returns the stored settings with the key values redacted by JSON.
func (s *ArtworkSettingsService) Get(ctx context.Context) (ArtworkSettings, error) {
	a, err := s.store.Get(ctx)
	if err != nil {
		return ArtworkSettings{}, err
	}
	return s.describe(a), nil
}

// ApplyStored loads the stored keys and applies them to the provider.
func (s *ArtworkSettingsService) ApplyStored(ctx context.Context) (ArtworkSettings, error) {
	a, err := s.store.Get(ctx)
	if err != nil {
		s.applier.SetKeys(s.envFanart, s.envAudioDB)
		return ArtworkSettings{}, err
	}
	s.apply(a)
	return s.describe(a), nil
}

// Update validates and saves the keys, then applies them.
func (s *ArtworkSettingsService) Update(ctx context.Context, upd ArtworkSettingsUpdate) (ArtworkSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.store.Get(ctx)
	if err != nil {
		return ArtworkSettings{}, err
	}
	next, err := resolveArtworkKeys(current, upd)
	if err != nil {
		return ArtworkSettings{}, err
	}
	if err := s.store.Save(ctx, next); err != nil {
		return ArtworkSettings{}, err
	}
	s.apply(next)
	return s.describe(next), nil
}

// Test checks unsaved keys. Empty keys reuse the effective ones so admins can
// test without re-entering secrets.
func (s *ArtworkSettingsService) Test(ctx context.Context, upd ArtworkSettingsUpdate) (ArtworkTestResult, error) {
	current, err := s.store.Get(ctx)
	if err != nil {
		return ArtworkTestResult{}, err
	}
	next, err := resolveArtworkKeys(current, upd)
	if err != nil {
		return ArtworkTestResult{}, err
	}
	fanart, audioDB := s.effective(next)
	fanartErr, audioDBErr := s.applier.TestKeys(ctx, fanart, audioDB)
	res := ArtworkTestResult{
		FanartTested:  fanart != "",
		FanartOK:      fanart != "" && fanartErr == nil,
		AudioDBOK:     audioDBErr == nil,
		AudioDBShared: audioDB == "",
	}
	if fanartErr != nil {
		res.FanartError = fanartErr.Error()
	}
	if audioDBErr != nil {
		res.AudioDBError = audioDBErr.Error()
	}
	res.OK = res.AudioDBOK && (!res.FanartTested || res.FanartOK)
	return res, nil
}

func (s *ArtworkSettingsService) apply(a ArtworkSettings) {
	fanart, audioDB := s.effective(a)
	s.applier.SetKeys(fanart, audioDB)
}

func (s *ArtworkSettingsService) effective(a ArtworkSettings) (fanart, audioDB string) {
	fanart, audioDB = a.FanartAPIKey, a.AudioDBAPIKey
	if fanart == "" {
		fanart = s.envFanart
	}
	if audioDB == "" {
		audioDB = s.envAudioDB
	}
	return fanart, audioDB
}

func (s *ArtworkSettingsService) describe(a ArtworkSettings) ArtworkSettings {
	a.FanartKeySet = a.FanartAPIKey != ""
	a.AudioDBKeySet = a.AudioDBAPIKey != ""
	a.FanartKeyFromEnv = !a.FanartKeySet && s.envFanart != ""
	a.AudioDBKeyFromEnv = !a.AudioDBKeySet && s.envAudioDB != ""
	return a
}

func resolveArtworkKeys(current ArtworkSettings, upd ArtworkSettingsUpdate) (ArtworkSettings, error) {
	next := ArtworkSettings{FanartAPIKey: current.FanartAPIKey, AudioDBAPIKey: current.AudioDBAPIKey}
	if upd.ClearFanartAPIKey {
		next.FanartAPIKey = ""
	} else if k := strings.TrimSpace(upd.FanartAPIKey); k != "" {
		next.FanartAPIKey = k
	}
	if upd.ClearAudioDBAPIKey {
		next.AudioDBAPIKey = ""
	} else if k := strings.TrimSpace(upd.AudioDBAPIKey); k != "" {
		next.AudioDBAPIKey = k
	}
	for name, k := range map[string]string{"fanart_api_key": next.FanartAPIKey, "audiodb_api_key": next.AudioDBAPIKey} {
		if len(k) > 256 || !validAPIKey(k) {
			return ArtworkSettings{}, fmt.Errorf("%w: %s must be printable characters without spaces (max 256)", ErrInvalid, name)
		}
	}
	return next, nil
}

func validAPIKey(k string) bool {
	for _, r := range k {
		if r <= ' ' || r > '~' {
			return false
		}
	}
	return true
}

// ArtworkSettingsRouter serves GET /, PUT / and POST /test. Mount it behind
// admin authorization: it manages third-party API keys.
func ArtworkSettingsRouter(svc *ArtworkSettingsService) chi.Router {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		a, err := svc.Get(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	})
	r.Put("/", func(w http.ResponseWriter, r *http.Request) {
		var upd ArtworkSettingsUpdate
		if err := decodeArtworkSettings(r, &upd); err != nil {
			writeErr(w, err)
			return
		}
		a, err := svc.Update(r.Context(), upd)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	})
	r.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		var upd ArtworkSettingsUpdate
		if err := decodeArtworkSettings(r, &upd); err != nil {
			writeErr(w, err)
			return
		}
		res, err := svc.Test(r.Context(), upd)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	return r
}

func decodeArtworkSettings(r *http.Request, upd *ArtworkSettingsUpdate) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(upd); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return nil
}
