package notifications

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router with all notification endpoints mounted.
func Router(service Service) chi.Router {
	r := chi.NewRouter()

	r.Get("/", listConnections(service))
	r.Post("/", createConnection(service))

	// Literal routes before /{id} wildcard
	r.Get("/history", listHistory(service))
	r.Post("/test", testConfig(service))

	r.Get("/{id}", getConnection(service))
	r.Put("/{id}", updateConnection(service))
	r.Delete("/{id}", deleteConnection(service))
	r.Post("/{id}/test", testConnection(service))

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

func listConnections(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conns, err := svc.ListConnections(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if conns == nil {
			conns = []*Connection{}
		}
		writeJSON(w, http.StatusOK, conns)
	}
}

func createConnection(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateConnectionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Type == "" {
			writeError(w, http.StatusBadRequest, "type is required")
			return
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		c := &Connection{
			Name:                req.Name,
			Type:                req.Type,
			Enabled:             enabled,
			Settings:            req.Settings,
			OnGrab:              req.OnGrab,
			OnDownload:          req.OnDownload,
			OnUpgrade:           req.OnUpgrade,
			OnRename:            req.OnRename,
			OnDelete:            req.OnDelete,
			OnHealthIssue:       req.OnHealthIssue,
			OnApplicationUpdate: req.OnApplicationUpdate,
			OnPlayback:          req.OnPlayback,
			Tags:                req.Tags,
		}

		if err := svc.CreateConnection(r.Context(), c); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, c)
	}
}

func getConnection(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		conn, err := svc.GetConnection(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, conn)
	}
}

func updateConnection(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		existing, err := svc.GetConnection(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		var req UpdateConnectionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if req.Name != nil {
			existing.Name = *req.Name
		}
		if req.Type != nil {
			existing.Type = *req.Type
		}
		if req.Enabled != nil {
			existing.Enabled = *req.Enabled
		}
		if req.Settings != nil {
			existing.Settings = *req.Settings
		}
		if req.OnGrab != nil {
			existing.OnGrab = *req.OnGrab
		}
		if req.OnDownload != nil {
			existing.OnDownload = *req.OnDownload
		}
		if req.OnUpgrade != nil {
			existing.OnUpgrade = *req.OnUpgrade
		}
		if req.OnRename != nil {
			existing.OnRename = *req.OnRename
		}
		if req.OnDelete != nil {
			existing.OnDelete = *req.OnDelete
		}
		if req.OnHealthIssue != nil {
			existing.OnHealthIssue = *req.OnHealthIssue
		}
		if req.OnApplicationUpdate != nil {
			existing.OnApplicationUpdate = *req.OnApplicationUpdate
		}
		if req.OnPlayback != nil {
			existing.OnPlayback = *req.OnPlayback
		}
		if req.Tags != nil {
			existing.Tags = req.Tags
		}

		if err := svc.UpdateConnection(r.Context(), existing); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, existing)
	}
}

func deleteConnection(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.DeleteConnection(r.Context(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func testConnection(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.TestConnection(r.Context(), id); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"message": "test notification sent successfully"})
	}
}

func testConfig(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateConnectionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if req.Type == "" {
			writeError(w, http.StatusBadRequest, "type is required")
			return
		}

		conn := &Connection{
			Name:     req.Name,
			Type:     req.Type,
			Settings: req.Settings,
		}

		sender := senderFor(conn.Type)
		testNotification := Notification{
			EventType: EventOnTest,
			Title:     "Test Notification",
			Message:   "This is a test notification from an unsaved connection.",
		}

		if err := sender.Send(r.Context(), testNotification, conn.Settings); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Test notification sent successfully"})
	}
}

func listHistory(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		entries, err := svc.ListHistory(r.Context(), limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if entries == nil {
			entries = []*HistoryEntry{}
		}
		writeJSON(w, http.StatusOK, entries)
	}
}
