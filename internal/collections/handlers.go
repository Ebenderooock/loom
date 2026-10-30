package collections

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/auth"
)

type endpoint func(http.ResponseWriter, *http.Request) (any, error)

func respond(fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := fn(w, r)
		if err != nil {
			status, message := http.StatusInternalServerError, "Collection operation failed; retry or check server/database availability."
			switch {
			case errors.Is(err, sql.ErrNoRows):
				status, message = http.StatusNotFound, "Collection or member not found."
			case errors.Is(err, ErrInvalid):
				status, message = http.StatusBadRequest, err.Error()
			case errors.Is(err, ErrConflict):
				status, message = http.StatusConflict, ErrConflict.Error()
			case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
				status, message = http.StatusGatewayTimeout, "Collection operation timed out; retry."
			}
			http.Error(w, message, status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

func readBody(w http.ResponseWriter, r *http.Request, body any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(body); err != nil {
		return ErrInvalid
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return ErrInvalid
	}
	return nil
}

// Reads may follow merge redirects, but stale writers must reload the retained
// collection even when its integer revision happens to equal the old one.
func (s *Service) getForWrite(ctx context.Context, id string) (*Collection, error) {
	c, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.ID != id {
		return nil, ErrConflict
	}
	return c, nil
}

// Router reads under the server's ordinary authenticated library access policy.
// All collection management is admin-only, including source-triggered imports.
func Router(s *Service) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := auth.IdentityFrom(r.Context())
			if identity == nil {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			if r.Method != http.MethodGet && !identity.HasRole("admin") {
				http.Error(w, "administrator required", http.StatusForbidden)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Get("/", respond(func(_ http.ResponseWriter, r *http.Request) (any, error) {
		items, err := s.store.List(r.Context())
		if err != nil {
			return nil, err
		}
		library, err := s.library(r.Context())
		if err != nil {
			return nil, err
		}
		for _, c := range items {
			if err := decorate(c, library, false); err != nil {
				return nil, err
			}
		}
		sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
		return map[string]any{"items": items}, nil
	}))
	r.Post("/", respond(func(w http.ResponseWriter, r *http.Request) (any, error) {
		var body struct {
			Name     string  `json:"name"`
			Overview string  `json:"overview"`
			Source   *Source `json:"source,omitempty"`
		}
		if err := readBody(w, r, &body); err != nil {
			return nil, err
		}
		if body.Source != nil {
			if body.Name != "" || body.Overview != "" {
				return nil, ErrInvalid
			}
			if body.Source.LastSync != "" || body.Source.Error != "" {
				return nil, ErrInvalid
			}
			return s.Import(r.Context(), *body.Source)
		}
		c := &Collection{Name: strings.TrimSpace(body.Name), Overview: body.Overview}
		return c, s.store.Save(r.Context(), c, true)
	}))
	r.Post("/discover", respond(func(_ http.ResponseWriter, r *http.Request) (any, error) {
		return s.RequestScan(r.Context())
	}))
	r.Get("/scan", respond(func(_ http.ResponseWriter, r *http.Request) (any, error) {
		return s.ScanStatus(r.Context())
	}))
	r.Get("/{id}", respond(func(_ http.ResponseWriter, r *http.Request) (any, error) {
		c, err := s.store.Get(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			return nil, err
		}
		library, err := s.library(r.Context())
		if err != nil {
			return nil, err
		}
		if err := decorate(c, library, true); err != nil {
			return nil, err
		}
		return c, nil
	}))
	r.Patch("/{id}", respond(func(w http.ResponseWriter, r *http.Request) (any, error) {
		var body struct {
			Name     string `json:"name"`
			Overview string `json:"overview"`
			Revision *int   `json:"revision"`
		}
		if err := readBody(w, r, &body); err != nil {
			return nil, err
		}
		c, err := s.getForWrite(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			return nil, err
		}
		if body.Revision == nil || *body.Revision < 0 {
			return nil, ErrInvalid
		}
		if *body.Revision != c.Revision {
			return nil, ErrConflict
		}
		c.Name, c.Overview = strings.TrimSpace(body.Name), body.Overview
		return c, s.store.Save(r.Context(), c, false)
	}))
	r.Post("/{id}/sources", respond(func(w http.ResponseWriter, r *http.Request) (any, error) {
		var ref Source
		if err := readBody(w, r, &ref); err != nil {
			return nil, err
		}
		if err := ref.Validate(); err != nil {
			return nil, err
		}
		if ref.LastSync != "" || ref.Error != "" {
			return nil, ErrInvalid
		}
		c, err := s.getForWrite(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			return nil, err
		}
		if owner, err := s.store.FindSource(r.Context(), ref); err == nil {
			if owner.ID != c.ID {
				return nil, ErrConflict
			}
			return c, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if len(c.Sources) >= 50 {
			return nil, ErrInvalid
		}
		c.Sources = append(c.Sources, ref)
		if err := s.Refresh(r.Context(), c); err != nil {
			return nil, err
		}
		return c, s.store.Save(r.Context(), c, false)
	}))
	r.Post("/{id}/refresh", respond(func(_ http.ResponseWriter, r *http.Request) (any, error) {
		c, err := s.getForWrite(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			return nil, err
		}
		if err := s.Refresh(r.Context(), c); err != nil {
			return nil, err
		}
		return c, s.store.Save(r.Context(), c, false)
	}))
	r.Post("/{id}/merge", respond(func(w http.ResponseWriter, r *http.Request) (any, error) {
		var body struct {
			FromID       string `json:"from_id"`
			Revision     *int   `json:"revision"`
			FromRevision *int   `json:"from_revision"`
		}
		if err := readBody(w, r, &body); err != nil {
			return nil, err
		}
		if body.Revision == nil || body.FromRevision == nil || *body.Revision < 0 || *body.FromRevision < 0 {
			return nil, ErrInvalid
		}
		if _, err := uuid.Parse(body.FromID); err != nil {
			return nil, ErrInvalid
		}
		into, err := s.getForWrite(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			return nil, err
		}
		from, err := s.getForWrite(r.Context(), body.FromID)
		if err != nil {
			return nil, err
		}
		if *body.Revision != into.Revision || *body.FromRevision != from.Revision {
			return nil, ErrConflict
		}
		return into, s.store.Merge(r.Context(), into, from)
	}))
	r.Post("/{id}/members", respond(func(w http.ResponseWriter, r *http.Request) (any, error) {
		var body struct {
			MediaType   string            `json:"media_type"`
			Title       string            `json:"title"`
			ExternalIDs map[string]string `json:"external_ids"`
		}
		if err := readBody(w, r, &body); err != nil {
			return nil, err
		}
		m := Member{ID: uuid.NewString(), MediaType: body.MediaType, Title: body.Title, ExternalIDs: body.ExternalIDs, Local: true}
		if err := m.Validate(); err != nil {
			return nil, err
		}
		// Local additions work without provider availability. They can be
		// enriched by a later source refresh sharing an exact external ID.
		c, err := s.getForWrite(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			return nil, err
		}
		c.Members, err = coalesce(c.Members, m)
		if err != nil {
			return nil, err
		}
		return c, s.store.Save(r.Context(), c, false)
	}))
	r.Patch("/{id}/members/{member}", respond(func(w http.ResponseWriter, r *http.Request) (any, error) {
		var body struct {
			Excluded *bool   `json:"excluded,omitempty"`
			Title    *string `json:"title,omitempty"`
		}
		if err := readBody(w, r, &body); err != nil {
			return nil, err
		}
		if body.Excluded == nil && body.Title == nil {
			return nil, ErrInvalid
		}
		c, err := s.getForWrite(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			return nil, err
		}
		for i := range c.Members {
			m := &c.Members[i]
			if m.ID != chi.URLParam(r, "member") {
				continue
			}
			if body.Excluded != nil {
				m.Excluded = *body.Excluded
			}
			if body.Title != nil {
				m.Title, m.Local = strings.TrimSpace(*body.Title), true
				if err := m.Validate(); err != nil {
					return nil, err
				}
			}
			return c, s.store.Save(r.Context(), c, false)
		}
		return nil, sql.ErrNoRows
	}))
	return r
}
