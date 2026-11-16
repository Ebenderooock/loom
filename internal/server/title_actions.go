package server

import (
	"context"
	"errors"
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
	FolderPath              *string `json:"folderPath"`
	UnavailableReason       *string `json:"unavailableReason"`
	RescanUnavailableReason *string `json:"rescanUnavailableReason"`
	PlexUnavailableReason   *string `json:"plexUnavailableReason"`
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
	r.With(admin).Post("/{id}/sync-plex", s.syncTitlePlex(mediaType))
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
			result.RescanUnavailableReason = &reason
			result.PlexUnavailableReason = &reason
			s.logger.WarnContext(r.Context(), "title action folder unavailable", "media_type", mediaType, "id", httpparam.Param(r, "id"))
		} else {
			result.FolderPath = &folder.Path
			if len(folder.Files) == 0 {
				reason := "No complete registered imported library files are available."
				result.RescanUnavailableReason = &reason
				result.PlexUnavailableReason = &reason
			}
		}
		if result.RescanUnavailableReason == nil &&
			((mediaType == "movie" && s.scannerSvc == nil) || (mediaType == "series" && s.seriesScannerSvc == nil)) {
			reason := "Library file scanning is not configured."
			result.RescanUnavailableReason = &reason
		}
		if result.PlexUnavailableReason == nil {
			if _, err := s.plexActionConnections(r.Context()); err != nil {
				reason := err.Error()
				result.PlexUnavailableReason = &reason
			}
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) plexActionConnections(ctx context.Context) ([]*connect.Connection, error) {
	if s.connectSvc == nil {
		return nil, errors.New("configure and enable a Plex connection in Settings > Connect")
	}
	connections, err := s.connectSvc.ListConnections(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "could not load Plex action connections")
		return nil, errors.New("could not load Plex connection settings")
	}
	var targets []*connect.Connection
	for _, c := range connections {
		if !c.Enabled || c.Provider != connect.ProviderPlex {
			continue
		}
		host, err := url.Parse(c.Settings.Host)
		if err != nil || host.Host == "" || (host.Scheme != "http" && host.Scheme != "https") ||
			host.User != nil || host.RawQuery != "" || host.Fragment != "" || strings.TrimSpace(c.Settings.APIKey) == "" {
			return nil, errors.New("an enabled Plex connection needs a valid server URL and token in Settings > Connect")
		}
		targets = append(targets, c)
	}
	if len(targets) == 0 {
		return nil, errors.New("configure and enable a Plex connection in Settings > Connect")
	}
	return targets, nil
}

func (s *Server) syncTitlePlex(mediaType string) http.HandlerFunc {
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
		targets, err := s.plexActionConnections(ctx)
		if err != nil {
			titleActionError(w, http.StatusConflict, err.Error())
			return
		}
		provider, err := connect.ProviderFor(connect.ProviderPlex)
		if err != nil {
			titleActionError(w, http.StatusInternalServerError, "Plex refresh is unavailable.")
			return
		}
		completed, failed := 0, 0
		for _, connection := range targets {
			// The provider refreshes libraries, not Loom paths. Tokens remain
			// server-side and never enter responses or error logs here.
			if err := provider.NotifyLibraryUpdate(ctx, connection.Settings); err != nil {
				failed++
				s.logger.WarnContext(ctx, "title Plex library refresh failed", "connection_id", connection.ID)
			} else {
				completed++
			}
		}
		if failed > 0 {
			titleActionError(w, http.StatusBadGateway, fmt.Sprintf(
				"Plex library refresh failed for %d connection(s); %d succeeded. Check the Plex server and connection settings.", failed, completed))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"message": "Plex library refresh requested", "connections": completed})
	}
}

func (s *Server) rescanTitleLibrary(mediaType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := httpparam.Param(r, "id")
		folder, err := s.actionFolder(r.Context(), mediaType, id)
		if err != nil {
			titleActionError(w, http.StatusConflict, titlefiles.ActionUnavailableReason(err))
			return
		}
		if len(folder.Files) == 0 {
			titleActionError(w, http.StatusConflict, "No complete registered imported library files are available.")
			return
		}
		var result *scanner.ScanResult
		switch {
		case mediaType == "movie" && s.scannerSvc != nil:
			result, err = s.scannerSvc.RescanRegisteredMovie(r.Context(), id, folder)
		case mediaType == "series" && s.seriesScannerSvc != nil:
			result, err = s.seriesScannerSvc.RescanRegisteredSeries(r.Context(), id, folder)
		default:
			titleActionError(w, http.StatusServiceUnavailable, "Library file scanning is not configured.")
			return
		}
		if err != nil {
			s.logger.ErrorContext(r.Context(), "registered file rescan failed", "media_type", mediaType, "id", id)
			titleActionError(w, http.StatusInternalServerError, "Could not check registered library files.")
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
