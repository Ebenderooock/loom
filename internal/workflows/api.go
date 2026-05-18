package workflows

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router for workflow API endpoints.
func Router(engine *Engine, orch *Orchestrator) chi.Router {
	h := &handler{engine: engine, orch: orch}
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Get("/{id}/events", h.listEvents)
	r.Post("/{id}/cancel", h.cancel)
	r.Post("/{id}/retry", h.retry)
	r.Delete("/{id}", h.delete)
	return r
}

type handler struct {
	engine *Engine
	orch   *Orchestrator
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	workflows, err := h.engine.store.ListRecent(r.Context(), 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if workflows == nil {
		workflows = []*Workflow{}
	}
	wfWriteJSON(w, workflows)
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	wf, err := h.engine.store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "workflow not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	wfWriteJSON(w, wf)
}

func (h *handler) listEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	events, err := h.engine.store.ListEvents(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []WorkflowEvent{}
	}
	wfWriteJSON(w, events)
}

func (h *handler) cancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.orch != nil {
		reply := make(chan error, 1)
		h.orch.Send(CmdCancel{WorkflowID: id, Reply: reply})
		if err := <-reply; err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if err := h.engine.Cancel(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	wf, _ := h.engine.store.Get(r.Context(), id)
	wfWriteJSON(w, wf)
}

func (h *handler) retry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.orch != nil {
		reply := make(chan error, 1)
		h.orch.Send(CmdRetry{WorkflowID: id, Reply: reply})
		if err := <-reply; err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if err := h.engine.Retry(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	wf, _ := h.engine.store.Get(r.Context(), id)
	wfWriteJSON(w, wf)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.engine.store.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func wfWriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
