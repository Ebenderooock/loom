// Package server wires the HTTP server: native /api/v1, wire-compat
// surfaces, /metrics, /healthz/readyz/livez, and (when built with the
// embed tag) the React app.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

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

// New constructs a Server but does not start listening.
func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	ctx := context.Background()

	tel, err := telemetry.New(ctx, cfg.Telemetry)
	if err != nil {
		return nil, fmt.Errorf("telemetry: %w", err)
	}

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
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("GET /api/v1/system/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"version":   buildinfo.Version,
			"commit":    buildinfo.Commit,
			"buildDate": buildinfo.Date,
			"engine":    s.db.Engine(),
		})
	})

	if h := s.tel.Handler(); h != nil {
		mux.Handle("GET /metrics", h)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	return s.withRequestLogging(mux)
}

func (s *Server) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		s.logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
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

// Shutdown stops the listener and releases resources.
func (s *Server) Shutdown(ctx context.Context) error {
	s.ready.Store(false)
	if err := s.httpSrv.Shutdown(ctx); err != nil {
		return err
	}
	if err := s.tel.Shutdown(ctx); err != nil {
		return err
	}
	return s.db.Close()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
