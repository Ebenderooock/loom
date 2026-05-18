package libraries

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router with all library endpoints mounted.
// Intended to be mounted at /api/v1/libraries.
func Router(store *Store, scanner *Scanner, logger *slog.Logger) chi.Router {
	r := chi.NewRouter()

	r.Get("/", listLibraries(store, logger))
	r.Post("/", createLibrary(store, logger))
	r.Get("/{id}", getLibrary(store, logger))
	r.Put("/{id}", updateLibrary(store, logger))
	r.Delete("/{id}", deleteLibrary(store, logger))
	r.Post("/{id}/scan", scanLibrary(store, scanner, logger))
	r.Get("/{id}/unmapped", listUnmapped(store, scanner, logger))

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    http.StatusText(status),
			"message": msg,
		},
	})
}

// enrichLibrary populates computed fields (disk space, file count, etc.).
func enrichLibrary(store *Store, l *Library, r *http.Request) {
	ctx := r.Context()

	// Check accessibility.
	if _, err := os.Stat(l.Path); err == nil {
		l.Accessible = true
	}

	// Disk space.
	if l.Accessible {
		if ds, err := GetDiskSpace(l.Path); err == nil {
			l.DiskSpace = ds
		}
	}

	// File count.
	if fc, err := store.FileCount(ctx, l.ID); err == nil {
		l.FileCount = fc
	}

	// Unmapped count.
	if uc, err := store.UnmappedCount(ctx, l.ID); err == nil {
		l.UnmappedCount = uc
	}
}

func listLibraries(store *Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		libs, err := store.List(r.Context())
		if err != nil {
			logger.Error("libraries: list", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if libs == nil {
			libs = []Library{}
		}
		for i := range libs {
			enrichLibrary(store, &libs[i], r)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": libs})
	}
}

func createLibrary(store *Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateLibraryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Path == "" {
			writeError(w, http.StatusBadRequest, "path is required")
			return
		}
		if req.MediaType == "" {
			req.MediaType = "movie"
		}

		monitorOnAdd := true
		if req.MonitorOnAdd != nil {
			monitorOnAdd = *req.MonitorOnAdd
		}

		qpID := req.QualityProfileID
		if qpID == "" {
			qpID = "default"
		}

		lib := &Library{
			Name:             req.Name,
			Path:             req.Path,
			MediaType:        req.MediaType,
			MonitorOnAdd:     monitorOnAdd,
			QualityProfileID: qpID,
		}
		if req.UnmonitorOnDelete != nil {
			lib.UnmonitorOnDelete = *req.UnmonitorOnDelete
		}
		if req.AutoArchiveWatched != nil {
			lib.AutoArchiveWatched = *req.AutoArchiveWatched
		}
		if req.AutoArchiveDaysAfterWatch != nil {
			lib.AutoArchiveDaysAfterWatch = *req.AutoArchiveDaysAfterWatch
		}

		if err := store.Create(r.Context(), lib); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				writeError(w, http.StatusConflict, "library path already exists")
				return
			}
			logger.Error("libraries: create", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		enrichLibrary(store, lib, r)
		writeJSON(w, http.StatusCreated, lib)
	}
}

func getLibrary(store *Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		lib, err := store.Get(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			logger.Error("libraries: get", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		enrichLibrary(store, lib, r)

		// Include files in detail view.
		files, err := store.ListFiles(r.Context(), id)
		if err != nil {
			logger.Error("libraries: list files", "err", err)
			files = []LibraryFile{}
		}
		if files == nil {
			files = []LibraryFile{}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"library": lib,
			"files":   files,
		})
	}
}

func updateLibrary(store *Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		lib, err := store.Get(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			logger.Error("libraries: get for update", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		var req UpdateLibraryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if req.Name != nil {
			lib.Name = *req.Name
		}
		if req.Path != nil {
			lib.Path = *req.Path
		}
		if req.MediaType != nil {
			lib.MediaType = *req.MediaType
		}
		if req.MonitorOnAdd != nil {
			lib.MonitorOnAdd = *req.MonitorOnAdd
		}
		if req.QualityProfileID != nil {
			lib.QualityProfileID = *req.QualityProfileID
		}

		if err := store.Update(r.Context(), lib); err != nil {
			logger.Error("libraries: update", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		enrichLibrary(store, lib, r)
		writeJSON(w, http.StatusOK, lib)
	}
}

func deleteLibrary(store *Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := store.Delete(r.Context(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			logger.Error("libraries: delete", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func scanLibrary(store *Store, scanner *Scanner, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		lib, err := store.Get(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			logger.Error("libraries: get for scan", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		// Run scan in background with a detached context so it
		// isn't cancelled when the HTTP handler returns 202.
		go func() {
			if err := scanner.ScanLibrary(context.Background(), lib); err != nil {
				logger.Error("libraries: scan failed", "id", id, "err", err)
			}
		}()

		writeJSON(w, http.StatusAccepted, map[string]any{
			"message": "scan started",
		})
	}
}

func listUnmapped(store *Store, scanner *Scanner, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		lib, err := store.Get(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			logger.Error("libraries: get for unmapped", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		folders, err := scanner.ListUnmappedFolders(r.Context(), lib)
		if err != nil {
			logger.Error("libraries: unmapped folders", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if folders == nil {
			folders = []UnmappedFolder{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": folders})
	}
}
