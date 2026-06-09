package bots

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/auth"
)

// httpHandlers serves the bot configuration + account-linking API.
type httpHandlers struct {
	store *Store
	sup   *Supervisor
	users UserDirectory
}

// Router exposes the bot endpoints, mounted under /api/v1/bots. The link
// preview/redeem endpoints require any authenticated user; configuration and
// link management are admin-only.
func Router(store *Store, sup *Supervisor, users UserDirectory, adminOnly func(http.Handler) http.Handler) chi.Router {
	h := &httpHandlers{store: store, sup: sup, users: users}
	r := chi.NewRouter()

	// Any authenticated user links their own account.
	r.Post("/link/preview", h.previewLink)
	r.Post("/link/redeem", h.redeemLink)

	// Admin only.
	r.Group(func(ar chi.Router) {
		ar.Use(adminOnly)
		ar.Get("/config", h.getConfig)
		ar.Put("/config", h.setConfig)
		ar.Get("/status", h.status)
		ar.Get("/links", h.listLinks)
		ar.Delete("/links/{id}", h.deleteLink)
	})
	return r
}

// configResponse masks secret tokens, reporting only whether each is set.
type configResponse struct {
	TelegramEnabled  bool `json:"telegram_enabled"`
	TelegramTokenSet bool `json:"telegram_token_set"`
	DiscordEnabled   bool `json:"discord_enabled"`
	DiscordTokenSet  bool `json:"discord_token_set"`

	DefaultMovieQualityProfileID  string `json:"default_movie_quality_profile_id"`
	DefaultMovieLibraryID         string `json:"default_movie_library_id"`
	DefaultSeriesQualityProfileID string `json:"default_series_quality_profile_id"`
	DefaultSeriesLibraryID        string `json:"default_series_library_id"`
	DefaultMusicQualityProfileID  string `json:"default_music_quality_profile_id"`
	DefaultMusicLibraryID         string `json:"default_music_library_id"`

	UpdatedAt time.Time `json:"updated_at"`
}

func toConfigResponse(c Config) configResponse {
	return configResponse{
		TelegramEnabled:               c.TelegramEnabled,
		TelegramTokenSet:              c.TelegramBotToken != "",
		DiscordEnabled:                c.DiscordEnabled,
		DiscordTokenSet:               c.DiscordBotToken != "",
		DefaultMovieQualityProfileID:  c.DefaultMovieQualityProfileID,
		DefaultMovieLibraryID:         c.DefaultMovieLibraryID,
		DefaultSeriesQualityProfileID: c.DefaultSeriesQualityProfileID,
		DefaultSeriesLibraryID:        c.DefaultSeriesLibraryID,
		DefaultMusicQualityProfileID:  c.DefaultMusicQualityProfileID,
		DefaultMusicLibraryID:         c.DefaultMusicLibraryID,
		UpdatedAt:                     c.UpdatedAt,
	}
}

func (h *httpHandlers) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.GetConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toConfigResponse(cfg))
}

// updateConfigRequest overlays partial changes. Token fields are pointers so
// nil = keep existing, "" = clear, and a value = replace.
type updateConfigRequest struct {
	TelegramEnabled  *bool   `json:"telegram_enabled"`
	TelegramBotToken *string `json:"telegram_bot_token"`
	DiscordEnabled   *bool   `json:"discord_enabled"`
	DiscordBotToken  *string `json:"discord_bot_token"`

	DefaultMovieQualityProfileID  *string `json:"default_movie_quality_profile_id"`
	DefaultMovieLibraryID         *string `json:"default_movie_library_id"`
	DefaultSeriesQualityProfileID *string `json:"default_series_quality_profile_id"`
	DefaultSeriesLibraryID        *string `json:"default_series_library_id"`
	DefaultMusicQualityProfileID  *string `json:"default_music_quality_profile_id"`
	DefaultMusicLibraryID         *string `json:"default_music_library_id"`
}

func (h *httpHandlers) setConfig(w http.ResponseWriter, r *http.Request) {
	var in updateConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cfg, err := h.store.GetConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if in.TelegramEnabled != nil {
		cfg.TelegramEnabled = *in.TelegramEnabled
	}
	if in.TelegramBotToken != nil {
		cfg.TelegramBotToken = *in.TelegramBotToken
	}
	if in.DiscordEnabled != nil {
		cfg.DiscordEnabled = *in.DiscordEnabled
	}
	if in.DiscordBotToken != nil {
		cfg.DiscordBotToken = *in.DiscordBotToken
	}
	if in.DefaultMovieQualityProfileID != nil {
		cfg.DefaultMovieQualityProfileID = *in.DefaultMovieQualityProfileID
	}
	if in.DefaultMovieLibraryID != nil {
		cfg.DefaultMovieLibraryID = *in.DefaultMovieLibraryID
	}
	if in.DefaultSeriesQualityProfileID != nil {
		cfg.DefaultSeriesQualityProfileID = *in.DefaultSeriesQualityProfileID
	}
	if in.DefaultSeriesLibraryID != nil {
		cfg.DefaultSeriesLibraryID = *in.DefaultSeriesLibraryID
	}
	if in.DefaultMusicQualityProfileID != nil {
		cfg.DefaultMusicQualityProfileID = *in.DefaultMusicQualityProfileID
	}
	if in.DefaultMusicLibraryID != nil {
		cfg.DefaultMusicLibraryID = *in.DefaultMusicLibraryID
	}

	// Reject enabling a platform without a token to fail fast in the UI.
	if cfg.TelegramEnabled && cfg.TelegramBotToken == "" {
		writeError(w, http.StatusBadRequest, "telegram is enabled but no bot token is set")
		return
	}
	if cfg.DiscordEnabled && cfg.DiscordBotToken == "" {
		writeError(w, http.StatusBadRequest, "discord is enabled but no bot token is set")
		return
	}

	if err := h.store.SetConfig(r.Context(), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.sup != nil {
		if err := h.sup.Reload(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "config saved but reload failed: "+err.Error())
			return
		}
	}
	cfg, _ = h.store.GetConfig(r.Context())
	writeJSON(w, http.StatusOK, toConfigResponse(cfg))
}

func (h *httpHandlers) status(w http.ResponseWriter, r *http.Request) {
	if h.sup == nil {
		writeJSON(w, http.StatusOK, []PlatformStatus{})
		return
	}
	writeJSON(w, http.StatusOK, h.sup.Status())
}

// linkResponse is an account link enriched with the Loom username.
type linkResponse struct {
	AccountLink
	Username string `json:"username"`
}

func (h *httpHandlers) listLinks(w http.ResponseWriter, r *http.Request) {
	links, err := h.store.ListLinks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]linkResponse, 0, len(links))
	for _, l := range links {
		name := ""
		if h.users != nil {
			if n, _, err := h.users.Lookup(r.Context(), l.UserID); err == nil {
				name = n
			}
		}
		out = append(out, linkResponse{AccountLink: l, Username: name})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *httpHandlers) deleteLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteLink(r.Context(), id); err != nil {
		if errors.Is(err, ErrLinkNotFound) {
			writeError(w, http.StatusNotFound, "link not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type codeRequest struct {
	Code string `json:"code"`
}

// previewLink returns the chat identity behind a code so the UI can confirm who
// is being linked before committing the bind.
func (h *httpHandlers) previewLink(w http.ResponseWriter, r *http.Request) {
	if auth.IdentityFrom(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var in codeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	lc, err := h.store.PreviewLinkCode(r.Context(), normalizeCode(in.Code))
	if err != nil {
		writeCodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"platform":          lc.Platform,
		"external_username": lc.ExternalUsername,
		"expires_at":        lc.ExpiresAt,
	})
}

// redeemLink binds the chat identity behind a code to the authenticated user.
func (h *httpHandlers) redeemLink(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	if id == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var in codeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	link, err := h.store.RedeemLinkCode(r.Context(), normalizeCode(in.Code), id.UserID)
	if err != nil {
		writeCodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func writeCodeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidCode):
		writeError(w, http.StatusNotFound, "invalid link code")
	case errors.Is(err, ErrCodeExpired):
		writeError(w, http.StatusGone, "link code expired — generate a new one with /link")
	case errors.Is(err, ErrLinkedToOther):
		writeError(w, http.StatusConflict, "that chat account is already linked to a different Loom user")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{"code": http.StatusText(status), "message": msg},
	})
}
