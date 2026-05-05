package importlists

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router with all import list endpoints mounted.
func Router(store *Store, syncMgr *SyncManager, logger *slog.Logger) chi.Router {
	r := chi.NewRouter()

	r.Get("/", listAllLists(store))
	r.Post("/", createList(store))

	// Literal routes before /{id}
	r.Get("/exclusions", listExclusions(store))
	r.Post("/exclusions", createExclusion(store))
	r.Delete("/exclusions/{id}", deleteExclusion(store))

	r.Get("/{id}", getList(store))
	r.Put("/{id}", updateList(store))
	r.Delete("/{id}", deleteList(store))
	r.Post("/{id}/sync", syncList(store, syncMgr, logger))

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func listAllLists(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lists, err := store.ListAll(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if lists == nil {
			lists = []*ImportList{}
		}

		type listWithCount struct {
			*ImportList
			ItemCount int `json:"item_count"`
		}
		var result []listWithCount
		for _, l := range lists {
			count, _ := store.ItemCount(r.Context(), l.ID)
			result = append(result, listWithCount{ImportList: l, ItemCount: count})
		}
		if result == nil {
			writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": result})
	}
}

func createList(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var l ImportList
		if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if l.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if l.ListType == "" {
			writeError(w, http.StatusBadRequest, "list_type is required")
			return
		}
		if l.MediaType == "" {
			l.MediaType = MediaTypeMovie
		}
		if l.MonitorType == "" {
			l.MonitorType = MonitorAll
		}
		if l.SyncIntervalMinutes == 0 {
			l.SyncIntervalMinutes = 360
		}
		if l.QualityProfileID == "" {
			l.QualityProfileID = "default"
		}

		if err := store.Create(r.Context(), &l); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, l)
	}
}

func getList(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		l, err := store.Get(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if l == nil {
			writeError(w, http.StatusNotFound, "list not found")
			return
		}

		items, err := store.ListItems(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if items == nil {
			items = []*ImportListItem{}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"list":  l,
			"items": items,
		})
	}
}

func updateList(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		existing, err := store.Get(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if existing == nil {
			writeError(w, http.StatusNotFound, "list not found")
			return
		}

		var update ImportList
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		// Merge fields
		update.ID = id
		if update.Name == "" {
			update.Name = existing.Name
		}
		if update.ListType == "" {
			update.ListType = existing.ListType
		}
		if update.MediaType == "" {
			update.MediaType = existing.MediaType
		}
		if update.MonitorType == "" {
			update.MonitorType = existing.MonitorType
		}
		if update.SyncIntervalMinutes == 0 {
			update.SyncIntervalMinutes = existing.SyncIntervalMinutes
		}
		if update.QualityProfileID == "" {
			update.QualityProfileID = existing.QualityProfileID
		}
		if update.Settings == "" {
			update.Settings = existing.Settings
		}

		if err := store.Update(r.Context(), &update); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, update)
	}
}

func deleteList(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := store.Delete(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func syncList(store *Store, syncMgr *SyncManager, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		l, err := store.Get(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if l == nil {
			writeError(w, http.StatusNotFound, "list not found")
			return
		}

		if err := syncMgr.SyncList(r.Context(), l); err != nil {
			logger.Error("import-lists: manual sync failed", "id", id, "err", err)
			writeError(w, http.StatusInternalServerError, "sync failed: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"message": "sync complete"})
	}
}

func listExclusions(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		exclusions, err := store.ListExclusions(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if exclusions == nil {
			exclusions = []*ImportListExclusion{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": exclusions})
	}
}

func createExclusion(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var e ImportListExclusion
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if e.Title == "" {
			writeError(w, http.StatusBadRequest, "title is required")
			return
		}
		if err := store.CreateExclusion(r.Context(), &e); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, e)
	}
}

func deleteExclusion(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := store.DeleteExclusion(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
