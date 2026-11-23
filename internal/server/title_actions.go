package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/connect"
	"github.com/ebenderooock/loom/internal/kernel/httpparam"
	"github.com/ebenderooock/loom/internal/scanner"
	"github.com/ebenderooock/loom/internal/titlefiles"
)

type titleActionAvailability struct {
	FolderPath                *string `json:"folderPath"`
	UnavailableReason         *string `json:"unavailableReason"`
	RescanUnavailableReason   *string `json:"rescanUnavailableReason"`
	PlexConfigured            bool    `json:"plexConfigured"`
	PlexUnavailableReason     *string `json:"plexUnavailableReason"`
	JellyfinConfigured        bool    `json:"jellyfinConfigured"`
	JellyfinUnavailableReason *string `json:"jellyfinUnavailableReason"`
}

func (s *Server) registerTitleActions(r chi.Router, mediaType string) {
	admin := func(next http.Handler) http.Handler {
		if s.authSvc == nil {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				titleActionError(w, http.StatusServiceUnavailable, "Administrator authentication is not configured.")
			})
		}
		return s.authSvc.RequireRole("admin")(next)
	}
	r.With(admin).Get("/{id}/actions", s.titleActions(mediaType))
	r.With(admin).Post("/{id}/sync-plex", s.syncTitleMediaServer(mediaType, connect.ProviderPlex, "Plex"))
	r.With(admin).Post("/{id}/sync-jellyfin", s.syncTitleMediaServer(mediaType, connect.ProviderJellyfin, "Jellyfin"))
	r.With(admin).Post("/{id}/rescan-library", s.rescanTitleLibrary(mediaType))
	r.With(admin).Post("/{id}/refresh-metadata", s.refreshTitleMetadata(mediaType))
}

func titleActionError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": message}})
}

func (s *Server) actionFolder(ctx context.Context, mediaType, id string) (titlefiles.ActionFolder, error) {
	if s.libStore == nil {
		return titlefiles.ActionFolder{}, fmt.Errorf("library configuration unavailable")
	}
	return titlefiles.New(s.db.DB(), s.libStore, s.appCfg, s.auditLog, nil).ResolveActionFolder(ctx, mediaType, id)
}

func (s *Server) titleActions(mediaType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var result titleActionAvailability
		folder, err := s.actionFolder(r.Context(), mediaType, httpparam.Param(r, "id"))
		if err != nil {
			reason := titlefiles.ActionUnavailableReason(err)
			result.UnavailableReason = &reason
			result.PlexUnavailableReason = &reason
			s.logger.WarnContext(r.Context(), "title action folder unavailable", "media_type", mediaType, "id", httpparam.Param(r, "id"))
		} else {
			result.FolderPath = &folder.Path
			if len(folder.Files) == 0 {
				reason := "No complete registered imported library files are available."
				result.PlexUnavailableReason = &reason
			}
		}
		if result.RescanUnavailableReason == nil &&
			((mediaType == "movie" && s.scannerSvc == nil) || (mediaType == "series" && s.seriesScannerSvc == nil)) {
			reason := "Library file scanning is not configured."
			result.RescanUnavailableReason = &reason
		}
		result.JellyfinUnavailableReason = result.PlexUnavailableReason
		if _, err := s.titleMediaServerConnections(r.Context(), connect.ProviderPlex, "Plex"); err != nil {
			reason := err.Error()
			result.PlexUnavailableReason = &reason
		} else {
			result.PlexConfigured = true
		}
		if _, err := s.titleMediaServerConnections(r.Context(), connect.ProviderJellyfin, "Jellyfin"); err != nil {
			reason := err.Error()
			result.JellyfinUnavailableReason = &reason
		} else {
			result.JellyfinConfigured = true
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) titleMediaServerConnections(ctx context.Context, providerType connect.ProviderType, name string) ([]*connect.Connection, error) {
	if s.connectSvc == nil {
		return nil, fmt.Errorf("configure and enable a %s connection in Settings > Connect", name)
	}
	connections, err := s.connectSvc.ListConnections(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "could not load title action connections", "provider", providerType)
		return nil, fmt.Errorf("could not load %s connection settings", name)
	}
	var targets []*connect.Connection
	for _, c := range connections {
		if !c.Enabled || c.Provider != providerType {
			continue
		}
		host, err := url.Parse(c.Settings.Host)
		if err != nil || host.Host == "" || (host.Scheme != "http" && host.Scheme != "https") ||
			host.User != nil || host.RawQuery != "" || host.Fragment != "" || strings.TrimSpace(c.Settings.APIKey) == "" {
			return nil, fmt.Errorf("an enabled %s connection needs a valid server URL and token in Settings > Connect", name)
		}
		targets = append(targets, c)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("configure and enable a %s connection in Settings > Connect", name)
	}
	return targets, nil
}

func (s *Server) syncTitleMediaServer(mediaType string, providerType connect.ProviderType, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
		defer cancel()
		folder, err := s.actionFolder(ctx, mediaType, httpparam.Param(r, "id"))
		if err != nil {
			titleActionError(w, http.StatusConflict, titlefiles.ActionUnavailableReason(err))
			return
		}
		if len(folder.Files) == 0 {
			titleActionError(w, http.StatusConflict, "No complete registered imported library files are available.")
			return
		}
		targets, err := s.titleMediaServerConnections(ctx, providerType, name)
		if err != nil {
			titleActionError(w, http.StatusConflict, err.Error())
			return
		}
		provider, err := connect.ProviderFor(providerType)
		if err != nil {
			titleActionError(w, http.StatusInternalServerError, name+" refresh is unavailable.")
			return
		}
		completed, failed := 0, 0
		for _, connection := range targets {
			// The provider refreshes libraries, not Loom paths. Tokens remain
			// server-side and never enter responses or error logs here.
			if err := provider.NotifyLibraryUpdate(ctx, connection.Settings); err != nil {
				failed++
				s.logger.WarnContext(ctx, "title media server library refresh failed", "provider", providerType, "connection_id", connection.ID)
			} else {
				completed++
			}
		}
		if failed > 0 {
			titleActionError(w, http.StatusBadGateway, fmt.Sprintf(
				"%s library refresh failed for %d connection(s); %d succeeded. Check the %s server and connection settings.", name, failed, completed, name))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"message": name + " library refresh requested", "connections": completed})
	}
}

func (s *Server) rescanTitleLibrary(mediaType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := httpparam.Param(r, "id")
		if s.libStore == nil {
			titleActionError(w, http.StatusServiceUnavailable, "Library configuration unavailable.")
			return
		}
		folder, err := titlefiles.New(s.db.DB(), s.libStore, s.appCfg, s.auditLog, nil).
			ResolveRescanFolder(r.Context(), mediaType, id)
		if err != nil {
			titleActionError(w, http.StatusConflict, titlefiles.ActionUnavailableReason(err))
			return
		}
		var result *scanner.ScanResult
		switch {
		case mediaType == "movie" && s.scannerSvc != nil:
			result, err = s.scannerSvc.RescanMovieFolder(r.Context(), id, folder)
		case mediaType == "series" && s.seriesScannerSvc != nil:
			result, err = s.seriesScannerSvc.RescanSeriesFolder(r.Context(), id, folder)
		default:
			titleActionError(w, http.StatusServiceUnavailable, "Library file scanning is not configured.")
			return
		}
		if err != nil {
			s.logger.ErrorContext(r.Context(), "title folder rescan failed", "media_type", mediaType, "id", id, "error", err)
			titleActionError(w, http.StatusInternalServerError, "Could not rescan the title's library folder.")
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) refreshTitleMetadata(mediaType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := httpparam.Param(r, "id")
		var err error
		if mediaType == "movie" {
			err = s.moviesSvc.RefreshMovie(r.Context(), id)
		} else {
			err = s.seriesSvc.RefreshSeriesMetadata(r.Context(), id)
		}
		if err != nil {
			s.logger.WarnContext(r.Context(), "title metadata refresh failed", "media_type", mediaType, "id", id)
			titleActionError(w, http.StatusBadGateway, "Metadata refresh failed. Check the title and metadata provider settings.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "Metadata refreshed"})
	}
}
