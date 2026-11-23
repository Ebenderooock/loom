package updates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/storage"
)

// Settings records the administrator's channel consent, not installation authority.
type Settings struct {
	Channel              string `json:"channel"`
	PreleaseOptIn        bool   `json:"preleaseOptIn"`
	AutoInstall          bool   `json:"autoInstall"`
	PostgresDedicatedAck bool   `json:"postgresDedicatedAck"`
}

// Keep the pre-existing settings record compatible with old binaries. The
// PostgreSQL acknowledgement is stored separately with its identity binding.
type storedSettings struct {
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
		return "PostgreSQL automatic installation is disabled pending completion and live validation of the updater candidate. Update notices remain available."
	}
	return "Automatic installation requires an installer-managed Linux/systemd or macOS/launchd SQLite service and its enrolled privileged helper. Other native installs remain manual-only; containers and Kubernetes use image/orchestration updates."
}

// Service persists update preferences independently of the YAML runtime config.
type Service struct {
	db         storage.DB
	checker    *Checker
	capability func() (bool, string)
	alerts     *Alerts
}

// NewService defaults to notices only until an installer capability is bound.
func NewService(db storage.DB, current string, commit ...string) *Service {
	return &Service{db: db, checker: NewChecker(current, commit...)}
}

// ConfigureInstallation binds eligibility to server-owned effective config and
// installer state, never to request fields or database edits.
func (s *Service) ConfigureInstallation(check func() (bool, string)) { s.capability = check }

// ConfigureAlerts enables notifications for verified update checks.
func (s *Service) ConfigureAlerts(alerts *Alerts) { s.alerts = alerts }

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
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return settings, err
	}
	if err == nil {
		if err := decodeStrict([]byte(raw), &settings); err != nil {
			return Settings{}, err
		}
		if err := settings.validate(); err != nil {
			return Settings{}, err
		}
	}
	// Ignore any acknowledgement embedded in the legacy settings record.
	settings.PostgresDedicatedAck = false
	if s.db.Engine() == storage.EnginePostgres {
		settings.PostgresDedicatedAck, err = s.postgresAcknowledged(ctx)
		if err != nil {
			return Settings{}, err
		}
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
	var ackRaw []byte
	if settings.PostgresDedicatedAck {
		fingerprint, ok := s.postgresIdentityFingerprint()
		if !ok {
			return errPostgresIdentityUnavailable
		}
		var err error
		ackRaw, err = json.Marshal(postgresAckRecord{Acknowledged: true, Identity: fingerprint})
		if err != nil {
			return err
		}
	}
	raw, err := json.Marshal(storedSettings{Channel: settings.Channel, PreleaseOptIn: settings.PreleaseOptIn, AutoInstall: settings.AutoInstall})
	if err != nil {
		return err
	}
	tx, err := s.db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO schema_meta (key, value) VALUES ('updates.settings', $1)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(raw))
	if err != nil {
		return err
	}
	if settings.PostgresDedicatedAck {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO schema_meta (key, value) VALUES ('updates.postgresDedicatedAck', $1)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(ackRaw))
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM schema_meta WHERE key = 'updates.postgresDedicatedAck'`)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
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
			"isPostgres":             s.db.Engine() == storage.EnginePostgres,
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
			if errors.Is(err, errPostgresIdentityUnavailable) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
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
		// force=true is the explicit "Check for updates" action: it bypasses the
		// hourly cache subject to the per-channel manual cooldown and GitHub
		// rate limits. Other callers keep the cached passive behaviour.
		var notice Notice
		if r.URL.Query().Get("force") == "true" {
			notice = s.checker.CheckNow(r.Context(), settings.Channel)
		} else {
			notice = s.checker.Check(r.Context(), settings.Channel)
		}
		if s.alerts != nil {
			if err := s.alerts.Available(r.Context(), notice); err != nil {
				slog.Error("update notification failed", "error", err)
			}
		}
		writeJSON(w, notice)
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
