package prowlarrv1

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/compat/syncprofiles"
	"github.com/ebenderooock/loom/internal/indexers"
)

// Handler serves the Prowlarr v1 compatibility API.
type Handler struct {
	svc       *indexers.Service
	syncStore *syncprofiles.Store
	logger    *slog.Logger
}

// NewHandler creates a Handler wired to the given indexer service.
// syncStore may be nil; when present the handler honours ?syncProfileId=
// on /indexer and /search to filter by sync-profile membership.
func NewHandler(svc *indexers.Service, syncStore *syncprofiles.Store, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		svc:       svc,
		syncStore: syncStore,
		logger:    logger.With("compat", "prowlarr/v1"),
	}
}

// Router returns a chi.Router with all Prowlarr v1 endpoints mounted.
func Router(h *Handler) chi.Router {
	r := chi.NewRouter()

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/indexer", h.listIndexers)
		r.Get("/indexer/{id}", h.getIndexer)
		r.Get("/search", h.search)
		r.Get("/indexerstats", h.indexerStats)
		r.Get("/tag", h.listTags)
		r.Get("/health", h.health)

		// Applications endpoints — Prowlarr exposes these so that
		// downstream apps (Radarr, Sonarr) can register themselves and
		// test connectivity. Loom acknowledges all operations successfully.
		r.Get("/applications", h.listApplications)
		r.Post("/applications", h.createApplication)
		r.Get("/applications/{id}", h.getApplication)
		r.Put("/applications/{id}", h.updateApplication)
		r.Delete("/applications/{id}", h.deleteApplication)
		r.Post("/applications/test", h.testApplication)
		r.Post("/applications/{id}/test", h.testApplication)
	})
	r.Get("/api/v1/system/status", h.systemStatus)

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

// --- endpoint handlers ---

func (h *Handler) listIndexers(w http.ResponseWriter, r *http.Request) {
	defs, err := h.svc.List(r.Context())
	if err != nil {
		h.logger.Error("list indexers", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Filter by sync profile if requested.
	allowed := h.allowedIndexerIDs(r)

	out := make([]prowlarrIndexer, 0, len(defs))
	for _, dh := range defs {
		if allowed != nil {
			if _, ok := allowed[dh.Definition.ID]; !ok {
				continue
			}
		}
		out = append(out, defToIndexer(dh.Definition))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) getIndexer(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")
	numID, err := intIDStr(idParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	defs, err := h.svc.List(r.Context())
	if err != nil {
		h.logger.Error("get indexer: list", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	for _, dh := range defs {
		if intID(dh.Definition.ID) == numID {
			// Check sync-profile membership.
			if allowed := h.allowedIndexerIDs(r); allowed != nil {
				if _, ok := allowed[dh.Definition.ID]; !ok {
					writeError(w, http.StatusNotFound, "indexer not found")
					return
				}
			}
			writeJSON(w, http.StatusOK, defToIndexer(dh.Definition))
			return
		}
	}
	writeError(w, http.StatusNotFound, "indexer not found")
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	q := indexers.Query{
		Term: r.URL.Query().Get("query"),
	}

	// Parse categories.
	if cats := r.URL.Query().Get("categories"); cats != "" {
		for _, s := range strings.Split(cats, ",") {
			s = strings.TrimSpace(s)
			if n, err := strconv.Atoi(s); err == nil {
				q.Categories = append(q.Categories, indexers.Category(n))
			}
		}
	}

	// Optional external IDs.
	if v := r.URL.Query().Get("imdbId"); v != "" {
		q.IMDBID = v
	}
	if v := r.URL.Query().Get("tvdbId"); v != "" {
		q.TVDBID = v
	}
	if v := r.URL.Query().Get("tmdbId"); v != "" {
		q.TMDBID = v
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Limit = n
		}
	}

	// Restrict to specific indexer IDs if given.
	var indexerIDs []string
	if idStr := r.URL.Query().Get("indexerIds"); idStr != "" {
		// Prowlarr sends numeric IDs; translate back to string UUIDs.
		defs, err := h.svc.List(r.Context())
		if err != nil {
			h.logger.Error("search: list for id mapping", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		pairs := make([]defPair, 0, len(defs))
		for _, dh := range defs {
			pairs = append(pairs, defPair{strID: dh.Definition.ID, numID: intID(dh.Definition.ID)})
		}
		for _, s := range strings.Split(idStr, ",") {
			s = strings.TrimSpace(s)
			if n, err := strconv.Atoi(s); err == nil {
				if uuid, ok := findIDByInt(pairs, n); ok {
					indexerIDs = append(indexerIDs, uuid)
				}
			}
		}
	}

	// Apply sync-profile filter on top of any explicit indexer ID list.
	if allowed := h.allowedIndexerIDs(r); allowed != nil {
		if len(indexerIDs) > 0 {
			filtered := indexerIDs[:0]
			for _, id := range indexerIDs {
				if _, ok := allowed[id]; ok {
					filtered = append(filtered, id)
				}
			}
			indexerIDs = filtered
		} else {
			indexerIDs = make([]string, 0, len(allowed))
			for id := range allowed {
				indexerIDs = append(indexerIDs, id)
			}
		}
	}

	agg := h.svc.Search(r.Context(), q, indexerIDs, 0)

	// Build a lookup from string indexer ID → numeric ID + protocol.
	type idInfo struct {
		numID    int
		protocol string
	}
	lookup := make(map[string]idInfo)
	if defs, err := h.svc.List(r.Context()); err == nil {
		for _, dh := range defs {
			lookup[dh.Definition.ID] = idInfo{
				numID:    intID(dh.Definition.ID),
				protocol: protocolFromKind(dh.Definition.Kind),
			}
		}
	}

	out := make([]prowlarrSearchResult, 0, len(agg.Results))
	for _, res := range agg.Results {
		info := lookup[res.IndexerID]
		// Infer protocol from result fields when definition lookup
		// didn't resolve.
		proto := info.protocol
		if proto == "" {
			if res.Seeders != nil {
				proto = "torrent"
			} else {
				proto = "usenet"
			}
		}
		out = append(out, resultToSearch(res, info.numID, proto))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) indexerStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, prowlarrIndexerStats{
		Indexers: []prowlarrIndexerStat{},
	})
}

func (h *Handler) systemStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, prowlarrSystemStatus{
		AppName:        "Loom (Prowlarr compat)",
		Version:        "1.0.0",
		BuildTime:      startTime.UTC().Format("2006-01-02T15:04:05Z"),
		IsProduction:   true,
		IsDocker:       false,
		Branch:         "main",
		Authentication: "none",
		RuntimeVersion: runtime.Version(),
		RuntimeName:    "go",
		StartTime:      startTime.UTC().Format("2006-01-02T15:04:05Z"),
		OsName:         runtime.GOOS,
	})
}

func (h *Handler) listTags(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []prowlarrHealth{})
}

// --- applications endpoints ---

// testApplication handles POST /api/v1/applications/test and
// POST /api/v1/applications/{id}/test.
// Prowlarr uses this to verify connectivity to downstream apps
// (Radarr, Sonarr). An empty array means "no validation errors" = success.
func (h *Handler) testApplication(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (h *Handler) listApplications(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []prowlarrApplication{})
}

func (h *Handler) createApplication(w http.ResponseWriter, r *http.Request) {
	var app prowlarrApplication
	if err := json.NewDecoder(r.Body).Decode(&app); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if app.ID == 0 {
		app.ID = 1
	}
	writeJSON(w, http.StatusCreated, app)
}

func (h *Handler) getApplication(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, prowlarrApplication{
		ID:   1,
		Name: "Loom",
	})
}

func (h *Handler) updateApplication(w http.ResponseWriter, r *http.Request) {
	var app prowlarrApplication
	if err := json.NewDecoder(r.Body).Decode(&app); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	writeJSON(w, http.StatusAccepted, app)
}

func (h *Handler) deleteApplication(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// allowedIndexerIDs returns a set of indexer IDs allowed by the sync
// profile specified via ?syncProfileId=. Returns nil when no filter
// is requested (all indexers visible).
func (h *Handler) allowedIndexerIDs(r *http.Request) map[string]struct{} {
	if h.syncStore == nil {
		return nil
	}
	profileID := r.URL.Query().Get("syncProfileId")
	if profileID == "" {
		return nil
	}
	ids, err := h.syncStore.FilteredIndexerIDs(r.Context(), profileID)
	if err != nil {
		h.logger.Warn("sync profile filter failed, allowing all", "profileId", profileID, "err", err)
		return nil
	}
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}
