// Package server wires the HTTP server: native /api/v1, wire-compat
// surfaces, /metrics, /healthz/readyz/livez, and (when built with the
// embed tag) the React app. Routing uses go-chi/chi/v5 with a standard
// middleware chain (request-ID, structured access log, panic recovery,
// gzip, ETag for system status, CORS).
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/loomctl/loom/internal/buildinfo"
	"github.com/loomctl/loom/internal/kernel/config"
	"github.com/loomctl/loom/internal/kernel/eventbus"
	"github.com/loomctl/loom/internal/kernel/telemetry"
	"github.com/loomctl/loom/internal/storage"
)

// Server holds wired dependencies for the HTTP listener.
type Server struct {
	cfg     *config.Config
	logger  *slog.Logger
	httpSrv *http.Server
	tel     *telemetry.Telemetry
	db      storage.DB
	bus     eventbus.Bus
	ready   atomic.Bool
}

// New constructs a Server but does not start listening. The caller must
// have already constructed *telemetry.Telemetry (typically via
// telemetry.Init in serve.go).
func New(cfg *config.Config, logger *slog.Logger, tel *telemetry.Telemetry) (*Server, error) {
	if tel == nil {
		return nil, errors.New("server: telemetry must not be nil")
	}
	ctx := context.Background()

	db, err := storage.Open(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}

	s := &Server{
		cfg:    cfg,
		logger: logger,
		tel:    tel,
		db:     db,
		bus:    eventbus.NewInProc(),
	}

	mux := s.newMux()

	s.httpSrv = &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           mux,
		ReadHeaderTimeout: time.Duration(cfg.HTTP.ReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(cfg.HTTP.WriteTimeout) * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	return s, nil
}

func (s *Server) newMux() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.accessLog)
	r.Use(s.recoverer)
	r.Use(middleware.Compress(5))

	if origins := s.cfg.CORS.AllowedOrigins; len(origins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   origins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Api-Key", "X-Request-Id"},
			ExposedHeaders:   []string{"X-Request-Id"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/livez", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !s.ready.Load() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "starting"})
			return
		}
		if err := s.db.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db unreachable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	r.Handle("/metrics", s.tel.Handler())

	r.Group(func(r chi.Router) {
		r.Use(etagMiddleware)
		r.Get("/api/v1/system/status", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{
				"version":   buildinfo.Version,
				"commit":    buildinfo.Commit,
				"buildDate": buildinfo.Date,
				"engine":    s.db.Engine(),
			})
		})
	})

	if s.cfg.Debug.Pprof {
		s.mountPprof(r)
	}

	return r
}

func (s *Server) mountPprof(r chi.Router) {
	r.Route("/debug/pprof", func(r chi.Router) {
		r.Get("/", pprof.Index)
		r.Get("/cmdline", pprof.Cmdline)
		r.Get("/profile", pprof.Profile)
		r.Post("/symbol", pprof.Symbol)
		r.Get("/symbol", pprof.Symbol)
		r.Get("/trace", pprof.Trace)
		r.Get("/{name}", func(w http.ResponseWriter, req *http.Request) {
			pprof.Handler(chi.URLParam(req, "name")).ServeHTTP(w, req)
		})
	})
}

// accessLog emits a structured slog record per request and propagates the
// chi-generated X-Request-Id back to the client.
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		reqID := middleware.GetReqID(r.Context())
		if reqID != "" {
			ww.Header().Set("X-Request-Id", reqID)
		}
		next.ServeHTTP(ww, r)
		s.logger.LogAttrs(r.Context(), slog.LevelInfo, "http",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", ww.Status()),
			slog.Int("bytes", ww.BytesWritten()),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("remote", r.RemoteAddr),
			slog.String("request_id", reqID),
		)
	})
}

// recoverer turns panics into a structured 500 JSON response and a stack
// trace logged at error level.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rv := recover(); rv != nil {
				if rv == http.ErrAbortHandler {
					panic(rv)
				}
				stack := debug.Stack()
				s.logger.Error("panic",
					"err", fmt.Sprintf("%v", rv),
					"path", r.URL.Path,
					"method", r.Method,
					"request_id", middleware.GetReqID(r.Context()),
					"stack", string(stack),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "internal server error",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// etagMiddleware buffers the response, hashes it, sets ETag, and replies
// with 304 if the client's If-None-Match matches. Designed for tiny GET
// endpoints (e.g. system status); not appropriate for large or streaming
// responses.
func etagMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		buf := &etagWriter{ResponseWriter: w, body: &bytes.Buffer{}, status: http.StatusOK}
		next.ServeHTTP(buf, r)

		sum := sha256.Sum256(buf.body.Bytes())
		etag := `"` + hex.EncodeToString(sum[:16]) + `"`
		w.Header().Set("ETag", etag)
		if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(buf.status)
		_, _ = w.Write(buf.body.Bytes())
	})
}

type etagWriter struct {
	http.ResponseWriter
	body        *bytes.Buffer
	status      int
	wroteHeader bool
}

func (e *etagWriter) WriteHeader(code int) {
	if !e.wroteHeader {
		e.status = code
		e.wroteHeader = true
	}
}

func (e *etagWriter) Write(b []byte) (int, error) {
	if !e.wroteHeader {
		e.WriteHeader(http.StatusOK)
	}
	return e.body.Write(b)
}

// Start begins serving and blocks until ListenAndServe returns.
func (s *Server) Start() error {
	s.ready.Store(true)
	s.logger.Info("listening", "addr", s.cfg.HTTP.Addr)
	if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops the listener and releases resources. Telemetry is owned
// by the caller (serve.go) and shut down separately.
func (s *Server) Shutdown(ctx context.Context) error {
	s.ready.Store(false)
	if err := s.httpSrv.Shutdown(ctx); err != nil {
		return err
	}
	return s.db.Close()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
