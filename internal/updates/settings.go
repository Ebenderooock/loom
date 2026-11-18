package updates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/storage"
)

// Settings records the administrator's channel consent, not installation authority.
type Settings struct {
	Channel       string `json:"channel"`
	PreleaseOptIn bool   `json:"preleaseOptIn"`
	AutoInstall   bool   `json:"autoInstall"`
}

func (s Settings) validate() error {
	if s.Channel != "stable" && s.Channel != "prelease" {
		return errors.New("channel must be stable or prelease")
	}
	if (s.Channel == "prelease") != s.PreleaseOptIn {
		return errors.New("prelease requires a separate explicit opt-in; clear it when returning to Stable")
	}
	return nil
}

// InstallationUnavailable is the default when no installer binding is wired.
// A writable directory or runtime preference never establishes eligibility.
func InstallationUnavailable(engine storage.Engine) string {
	if engine == storage.EnginePostgres {
		return "PostgreSQL installations are manual-update only. Update notices do not enable automatic installation."
	}
	return "Automatic installation requires an installer-managed Linux/systemd or macOS/launchd SQLite service and its enrolled privileged helper. Other native installs remain manual-only; containers and Kubernetes use image/orchestration updates."
}

// Service persists update preferences independently of the YAML runtime config.
type Service struct {
	db         storage.DB
	checker    *Checker
	capability func() (bool, string)
}

// NewService defaults to notices only until an installer capability is bound.
func NewService(db storage.DB, current string, commit ...string) *Service {
	return &Service{db: db, checker: NewChecker(current, commit...)}
}

// ConfigureInstallation binds eligibility to server-owned effective config and
// installer state, never to request fields or database edits.
func (s *Service) ConfigureInstallation(check func() (bool, string)) { s.capability = check }

func (s *Service) installation() (bool, string) {
	if s.db.Engine() != storage.EngineSQLite {
		return false, InstallationUnavailable(s.db.Engine())
	}
	if s.capability == nil {
		return false, InstallationUnavailable(s.db.Engine())
	}
	ok, reason := s.capability()
	if !ok {
		return false, reason
	}
	if !s.checker.Configured() {
		return false, "The publisher public key must be provisioned in both Loom and its privileged helper before automatic installation can be enabled."
	}
	return true, reason
}

// NativeSettings returns effective settings for the authenticated native helper.
func (s *Service) NativeSettings(ctx context.Context) (Settings, error) { return s.get(ctx) }

func (s *Service) get(ctx context.Context) (Settings, error) {
	settings := Settings{Channel: "stable"}
	var raw string
	err := s.db.DB().QueryRowContext(ctx, `SELECT value FROM schema_meta WHERE key = 'updates.settings'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if err := decodeStrict([]byte(raw), &settings); err != nil {
		return Settings{}, err
	}
	if err := settings.validate(); err != nil {
		return Settings{}, err
	}
	// A DB edit or a setting from a future build cannot enable installation.
	if ok, _ := s.installation(); !ok {
		settings.AutoInstall = false
	}
	return settings, nil
}

func (s *Service) set(ctx context.Context, settings Settings) error {
	if err := settings.validate(); err != nil {
		return err
	}
	if settings.AutoInstall {
		if ok, reason := s.installation(); !ok {
			return errors.New(reason)
		}
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.DB().ExecContext(ctx, storage.RebindQuery(
		`INSERT INTO schema_meta (key, value) VALUES ('updates.settings', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, s.db.Engine() == storage.EnginePostgres), string(raw))
	return err
}

// Router requires administrator authorization on all update settings and checks.
func (s *Service) Router(admin func(http.Handler) http.Handler) http.Handler {
	r := chi.NewRouter()
	// Unlike non-sensitive read-only settings, even network checks require an
	// administrator. Fail closed if auth has not been wired.
	if admin == nil {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "administrator authentication required", http.StatusForbidden)
			})
		})
	} else {
		r.Use(admin)
	}
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		settings, err := s.get(r.Context())
		if err != nil {
			http.Error(w, "could not load update settings", http.StatusInternalServerError)
			return
		}
		canInstall, reason := s.installation()
		writeJSON(w, map[string]any{
			"settings": settings, "canAutoInstall": canInstall,
			"installationNotice":     reason,
			"verificationConfigured": len(s.checker.keys) != 0,
		})
	})
	r.Put("/", func(w http.ResponseWriter, r *http.Request) {
		var settings Settings
		raw := http.MaxBytesReader(w, r.Body, 4096)
		d := json.NewDecoder(raw)
		d.DisallowUnknownFields()
		if err := d.Decode(&settings); err != nil || !errors.Is(d.Decode(new(any)), io.EOF) {
			http.Error(w, "invalid update settings", http.StatusBadRequest)
			return
		}
		if err := settings.validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if settings.AutoInstall {
			if ok, reason := s.installation(); !ok {
				http.Error(w, reason, http.StatusConflict)
				return
			}
		}
		if err := s.set(r.Context(), settings); err != nil {
			http.Error(w, "could not save update settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, settings)
	})
	r.Post("/check", func(w http.ResponseWriter, r *http.Request) {
		settings, err := s.get(r.Context())
		if err != nil {
			http.Error(w, "could not load update settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, s.checker.Check(r.Context(), settings.Channel))
	})
	return r
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		// The response may already have been written; never append an error body.
		return
	}
}
