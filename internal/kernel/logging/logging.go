// Package logging wires structured JSON logging via log/slog with PII redaction.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/loomctl/loom/internal/kernel/config"
)

// Sensitive keys whose values are redacted before emission.
var sensitiveKeys = map[string]struct{}{
	"password":      {},
	"passwd":        {},
	"secret":        {},
	"token":         {},
	"api_key":       {},
	"apikey":        {},
	"authorization": {},
	"cookie":        {},
	"set-cookie":    {},
}

// New returns a configured *slog.Logger.
func New(cfg config.LogConfig) (*slog.Logger, error) {
	return newWith(os.Stdout, cfg)
}

func newWith(w io.Writer, cfg config.LogConfig) (*slog.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: level <= slog.LevelDebug,
	}

	var base slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "text":
		base = slog.NewTextHandler(w, opts)
	default:
		base = slog.NewJSONHandler(w, opts)
	}

	return slog.New(&redactingHandler{inner: base}), nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("unknown log level %q", s)
}

// redactingHandler wraps another handler and replaces sensitive attribute
// values with the literal string "[REDACTED]".
type redactingHandler struct {
	inner slog.Handler
}

func (h *redactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	rr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		rr.AddAttrs(redact(a))
		return true
	})
	return h.inner.Handle(ctx, rr)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = redact(a)
	}
	return &redactingHandler{inner: h.inner.WithAttrs(out)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{inner: h.inner.WithGroup(name)}
}

func redact(a slog.Attr) slog.Attr {
	if _, ok := sensitiveKeys[strings.ToLower(a.Key)]; ok {
		return slog.String(a.Key, "[REDACTED]")
	}
	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		out := make([]slog.Attr, len(group))
		for i, sub := range group {
			out[i] = redact(sub)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	}
	return a
}
