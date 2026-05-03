package newznabserver

import (
	"context"
	"encoding/xml"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/loomctl/loom/internal/indexers"
)

// APIKeyVerifier is the small contract the aggregator needs from the
// auth layer: turn a presented API key into "valid" or "not valid"
// without revealing identity details. Any non-nil error is treated as
// "rejected"; the handler does not differentiate between expired,
// disabled, and unknown so as not to leak credential state.
type APIKeyVerifier interface {
	VerifyAPIKey(ctx context.Context, presentedKey string) error
}

// Searcher is the slice of indexers.Service the aggregator consumes.
// Defining it as an interface keeps server_test.go honest: tests can
// supply a fake without standing up a Repository.
type Searcher interface {
	Search(ctx context.Context, q indexers.Query, ids []string, perTimeout time.Duration) indexers.AggregatedResults
	Registry() *indexers.Registry
}

// Options configures NewServer.
type Options struct {
	// Search is the fan-out backend. Required.
	Search Searcher

	// Auth validates the apikey query parameter. Optional: when nil
	// the handler runs in "auth disabled" mode and accepts every
	// request, matching the rest of Loom's behaviour when the auth
	// service is itself disabled.
	Auth APIKeyVerifier

	// Logger receives structured logs. Defaults to slog.Default().
	Logger *slog.Logger

	// PerIndexerTimeout bounds how long a single upstream may take
	// before its result is dropped. Zero falls back to the parent
	// indexers.Service default.
	PerIndexerTimeout time.Duration

	// Title and Strapline appear on the <server> element of caps
	// documents. They are advisory; clients show them in admin UIs.
	Title     string
	Strapline string
}

// Server is the Newznab/Torznab aggregator HTTP surface. Construct
// with NewServer and attach to a chi.Router via Mount.
type Server struct {
	search    Searcher
	auth      APIKeyVerifier
	logger    *slog.Logger
	timeout   time.Duration
	title     string
	strapline string
}

// NewServer validates opts and returns a wired Server. Search is the
// only mandatory option; everything else has a safe default.
func NewServer(opts Options) (*Server, error) {
	if opts.Search == nil {
		return nil, errors.New("newznabserver: search backend is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = "Loom"
	}
	strapline := strings.TrimSpace(opts.Strapline)
	if strapline == "" {
		strapline = "Loom Newznab/Torznab aggregator"
	}
	return &Server{
		search:    opts.Search,
		auth:      opts.Auth,
		logger:    logger.With("module", "newznabserver"),
		timeout:   opts.PerIndexerTimeout,
		title:     title,
		strapline: strapline,
	}, nil
}

// Mount registers the aggregator routes on r. Both the canonical
// `/api` path (what Prowlarr-aware clients expect) and the explicit
// `/api/v1/aggregate` alias resolve to the same handler. ADR-0011
// records why we ship both.
func (s *Server) Mount(r chi.Router) {
	r.Get("/api", s.handle)
	r.Get("/api/v1/aggregate", s.handle)
}

// handle dispatches on the `t=` query parameter. Unknown modes get a
// Newznab "Function not implemented" error rather than a 404 so the
// client surfaces a helpful message.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if err := s.checkAuth(r); err != nil {
		writeError(w, http.StatusUnauthorized, errCodeAPIKeyInvalid, err.Error())
		return
	}
	mode := strings.TrimSpace(r.URL.Query().Get("t"))
	if mode == "" {
		writeError(w, http.StatusBadRequest, errCodeMissingParameter,
			"missing required parameter: t")
		return
	}
	switch mode {
	case "caps":
		s.handleCaps(w, r)
	case "search":
		s.handleSearch(w, r, indexers.Query{})
	case "movie":
		s.handleMovie(w, r)
	case "tvsearch":
		s.handleTVSearch(w, r)
	case "music":
		s.handleSearch(w, r, indexers.Query{
			Categories: []indexers.Category{indexers.CategoryAudio},
		})
	case "book":
		s.handleSearch(w, r, indexers.Query{
			Categories: []indexers.Category{indexers.CategoryBooks},
		})
	default:
		writeError(w, http.StatusBadRequest, errCodeFunctionNotImpl,
			"function not implemented: t="+mode)
	}
}

// checkAuth resolves the apikey query parameter against the
// configured verifier. When no verifier is wired (auth disabled at
// the project level) every request passes; this matches the rest of
// Loom's "RequireAuth is a no-op when auth.mode=disabled" stance.
func (s *Server) checkAuth(r *http.Request) error {
	if s.auth == nil {
		return nil
	}
	key := strings.TrimSpace(r.URL.Query().Get("apikey"))
	if key == "" {
		// Some clients send the key in X-Api-Key instead; honour
		// that too for parity with the JSON API.
		key = strings.TrimSpace(r.Header.Get("X-Api-Key"))
	}
	if key == "" {
		return errors.New("missing apikey query parameter")
	}
	if err := s.auth.VerifyAPIKey(r.Context(), key); err != nil {
		return errors.New("invalid apikey")
	}
	return nil
}

// --- caps -----------------------------------------------------------

func (s *Server) handleCaps(w http.ResponseWriter, r *http.Request) {
	all := s.search.Registry().List()
	doc := aggregateCaps(all, s.title, s.strapline)
	writeXML(w, doc)
	s.logger.Debug("caps served",
		"indexers", len(all),
		"categories", len(doc.Categories.Categories),
	)
}

// --- search variants ------------------------------------------------

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request, base indexers.Query) {
	q := buildBaseQuery(r, base)
	s.runSearch(w, r, q)
}

func (s *Server) handleMovie(w http.ResponseWriter, r *http.Request) {
	q := buildBaseQuery(r, indexers.Query{})
	if id := strings.TrimSpace(r.URL.Query().Get("imdbid")); id != "" {
		q.IMDBID = id
	}
	if id := strings.TrimSpace(r.URL.Query().Get("tmdbid")); id != "" {
		q.TMDBID = id
	}
	if len(q.Categories) == 0 {
		q.Categories = []indexers.Category{indexers.CategoryMovies}
	}
	s.runSearch(w, r, q)
}

func (s *Server) handleTVSearch(w http.ResponseWriter, r *http.Request) {
	q := buildBaseQuery(r, indexers.Query{})
	if id := strings.TrimSpace(r.URL.Query().Get("tvdbid")); id != "" {
		q.TVDBID = id
	}
	if v := strings.TrimSpace(r.URL.Query().Get("season")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Season = n
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("ep")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Episode = n
		}
	}
	if len(q.Categories) == 0 {
		q.Categories = []indexers.Category{indexers.CategoryTV}
	}
	s.runSearch(w, r, q)
}

// buildBaseQuery extracts the parameters every Newznab search mode
// shares (q, cat, limit, offset). Caller-supplied seed values
// (categories from a mode-specific dispatcher) are preserved.
func buildBaseQuery(r *http.Request, seed indexers.Query) indexers.Query {
	q := seed
	if v := strings.TrimSpace(r.URL.Query().Get("q")); v != "" {
		q.Term = v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("cat")); v != "" {
		if cats := parseCategoryList(v); len(cats) > 0 {
			q.Categories = cats
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q.Limit = n
		}
	}
	return q
}

// parseCategoryList splits a comma-separated list of integer IDs. Any
// non-numeric token is silently dropped — Newznab clients sometimes
// send "all" or human-friendly labels and we don't want to 400 on
// those.
func parseCategoryList(raw string) []indexers.Category {
	parts := strings.Split(raw, ",")
	out := make([]indexers.Category, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			out = append(out, indexers.Category(n))
		}
	}
	return out
}

// runSearch fans the query out, sorts the results newest-first and
// renders the RSS document. Per-source errors are logged but never
// surfaced to the client: Newznab has no concept of partial-success
// errors, and Prowlarr clients (Sonarr, Radarr) treat any 5xx as
// "indexer down" and back off.
func (s *Server) runSearch(w http.ResponseWriter, r *http.Request, q indexers.Query) {
	agg := s.search.Search(r.Context(), q, nil, s.timeout)
	if len(agg.Errors) > 0 {
		s.logger.Warn("aggregator partial failure",
			"errors", agg.Errors,
			"results", len(agg.Results),
			"q", q.Term,
		)
	}
	sortResultsByPubDate(agg.Results)
	feed := renderFeed(agg.Results, feedOptions{
		Title:       s.title,
		Description: s.strapline,
		Link:        derivePublicLink(r),
		SelfURL:     r.URL.String(),
	})
	writeXML(w, feed)
}

func sortResultsByPubDate(rows []indexers.Result) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].PubDate.After(rows[j].PubDate)
	})
}

// derivePublicLink returns a best-effort base URL for the channel's
// `<link>` element. The exact value is informational; we use the
// scheme+host the request arrived on so the link matches whatever
// reverse proxy fronts Loom.
func derivePublicLink(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

// writeXML emits the XML prologue plus the marshalled document.
// Callers pass already-typed structs; we never marshal arbitrary
// values so encoding errors are practically impossible — but we still
// log if one slips through rather than silently writing a blank body.
func writeXML(w http.ResponseWriter, doc any) {
	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		// Fall back to a minimal valid error document.
		writeError(w, http.StatusInternalServerError, errCodeInternal,
			"internal error rendering response")
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
}
