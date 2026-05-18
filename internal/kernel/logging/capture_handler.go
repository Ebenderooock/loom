package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// WorkflowIDExtractor is a function that extracts a workflow ID from context.
// This avoids a hard dependency on the workflows package.
type WorkflowIDExtractor func(ctx context.Context) (string, bool)

// CaptureHandler is an slog.Handler that tees log records to a ring buffer
// and an optional database sink, in addition to the wrapped console handler.
// It extracts workflow IDs from context and slog attributes for correlation.
type CaptureHandler struct {
	inner         slog.Handler
	buffer        *RingBuffer
	dbSink        chan<- LogEntry
	captureLevel  atomic.Int64
	extractWfID   WorkflowIDExtractor
	groups        []string
	presetAttrs   []slog.Attr
}

// CaptureHandlerConfig configures the capture handler.
type CaptureHandlerConfig struct {
	// Inner is the wrapped handler that still writes to console.
	Inner slog.Handler
	// Buffer is the in-memory ring buffer.
	Buffer *RingBuffer
	// DBSink is an optional channel for async DB persistence. May be nil.
	DBSink chan<- LogEntry
	// CaptureLevel is the minimum level to capture (independent of console level).
	CaptureLevel slog.Level
	// ExtractWorkflowID extracts a workflow ID from context. May be nil.
	ExtractWorkflowID WorkflowIDExtractor
}

// NewCaptureHandler creates a handler that tees to ring buffer + DB.
func NewCaptureHandler(cfg CaptureHandlerConfig) *CaptureHandler {
	h := &CaptureHandler{
		inner:       cfg.Inner,
		buffer:      cfg.Buffer,
		dbSink:      cfg.DBSink,
		extractWfID: cfg.ExtractWorkflowID,
	}
	h.captureLevel.Store(int64(cfg.CaptureLevel))
	return h
}

// SetCaptureLevel changes the minimum capture level at runtime.
func (h *CaptureHandler) SetCaptureLevel(level slog.Level) {
	h.captureLevel.Store(int64(level))
}

// CaptureLevel returns the current capture level.
func (h *CaptureHandler) GetCaptureLevel() slog.Level {
	return slog.Level(h.captureLevel.Load())
}

func (h *CaptureHandler) Enabled(ctx context.Context, level slog.Level) bool {
	captureEnabled := level >= slog.Level(h.captureLevel.Load())
	consoleEnabled := h.inner.Enabled(ctx, level)
	return captureEnabled || consoleEnabled
}

func (h *CaptureHandler) Handle(ctx context.Context, r slog.Record) error {
	// Always forward to the console handler if it accepts this level.
	if h.inner.Enabled(ctx, r.Level) {
		if err := h.inner.Handle(ctx, r); err != nil {
			return err
		}
	}

	// Capture to ring buffer / DB if at or above capture level.
	if r.Level < slog.Level(h.captureLevel.Load()) {
		return nil
	}

	entry := h.recordToEntry(ctx, r)

	if h.buffer != nil {
		h.buffer.Write(entry)
	}

	if h.dbSink != nil {
		select {
		case h.dbSink <- entry:
		default:
			// Drop on full channel to avoid blocking the logger.
		}
	}

	return nil
}

func (h *CaptureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := &CaptureHandler{
		inner:       h.inner.WithAttrs(attrs),
		buffer:      h.buffer,
		dbSink:      h.dbSink,
		extractWfID: h.extractWfID,
		groups:      h.groups,
		presetAttrs: append(cloneAttrs(h.presetAttrs), attrs...),
	}
	clone.captureLevel.Store(h.captureLevel.Load())
	return clone
}

func (h *CaptureHandler) WithGroup(name string) slog.Handler {
	clone := &CaptureHandler{
		inner:       h.inner.WithGroup(name),
		buffer:      h.buffer,
		dbSink:      h.dbSink,
		extractWfID: h.extractWfID,
		groups:      append(append([]string{}, h.groups...), name),
		presetAttrs: cloneAttrs(h.presetAttrs),
	}
	clone.captureLevel.Store(h.captureLevel.Load())
	return clone
}

func (h *CaptureHandler) recordToEntry(ctx context.Context, r slog.Record) LogEntry {
	entry := LogEntry{
		ID:        uuid.NewString(),
		Timestamp: r.Time.UTC().Format(time.RFC3339Nano),
		Level:     levelString(r.Level),
		Message:   r.Message,
	}

	// Source info.
	if r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		if f.File != "" {
			entry.Source = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
	}

	// Extract workflow ID from context first.
	if h.extractWfID != nil {
		if wfID, ok := h.extractWfID(ctx); ok {
			entry.WorkflowID = wfID
		}
	}

	// Collect attributes (preset + record).
	attrs := make(map[string]any)
	for _, a := range h.presetAttrs {
		flattenAttr(attrs, "", a)
	}
	r.Attrs(func(a slog.Attr) bool {
		flattenAttr(attrs, "", a)
		return true
	})

	// Fallback: extract workflow_id from attrs if not already set.
	if entry.WorkflowID == "" {
		for _, key := range []string{"workflow_id", "workflowID", "workflowId"} {
			if v, ok := attrs[key]; ok {
				entry.WorkflowID = fmt.Sprint(v)
				break
			}
		}
	}

	if len(attrs) > 0 {
		b, err := json.Marshal(attrs)
		if err == nil {
			entry.Attrs = string(b)
		}
	}

	return entry
}

func flattenAttr(m map[string]any, prefix string, a slog.Attr) {
	key := a.Key
	if prefix != "" {
		key = prefix + "." + key
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, sub := range a.Value.Group() {
			flattenAttr(m, key, sub)
		}
		return
	}
	m[key] = a.Value.Any()
}

func cloneAttrs(attrs []slog.Attr) []slog.Attr {
	if attrs == nil {
		return nil
	}
	out := make([]slog.Attr, len(attrs))
	copy(out, attrs)
	return out
}

func levelString(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	case l >= slog.LevelInfo:
		return "info"
	default:
		return "debug"
	}
}

// ParseCaptureLevel converts a string level name to slog.Level.
func ParseCaptureLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown capture level %q", s)
	}
}
