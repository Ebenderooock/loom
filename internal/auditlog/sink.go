// Package auditlog/sink provides an event bus subscriber that projects
// domain events into the centralized audit_log table. Each event type
// is mapped to a category, event_type, level, and a structured detail
// JSON payload for drill-down.
package auditlog

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
)

// AuditableTopics is the exhaustive list of event bus topics the sink
// subscribes to. Adding a new domain event? Register its topic here
// and add a mapping in eventToEntry.
var AuditableTopics = []string{
	"downloads.queued",
	"downloads.failed",
	"downloads.completed",
	"downloads.stalled",
	"downloads.retry",
	"imports.completed",
	"imports.failed",
	"search.completed",
	"search.failed",
	"scan.imported",
}

// Sink listens on the event bus and projects every auditable event
// into the audit_log table. It uses LogBackground so HTTP request
// context cancellation cannot prevent the write.
type Sink struct {
	logger     *Logger
	slogger    *slog.Logger
	unsubs     []func()
}

// NewSink creates a Sink and subscribes to all auditable topics.
func NewSink(bus eventbus.Bus, logger *Logger, slogger *slog.Logger) *Sink {
	s := &Sink{logger: logger, slogger: slogger}
	for _, topic := range AuditableTopics {
		unsub := bus.Subscribe(topic, s.handle)
		s.unsubs = append(s.unsubs, unsub)
	}
	return s
}

// Close unsubscribes from all topics.
func (s *Sink) Close() {
	for _, fn := range s.unsubs {
		fn()
	}
	s.unsubs = nil
}

func (s *Sink) handle(_ context.Context, ev eventbus.Event) error {
	entry := eventToEntry(ev)
	if entry == nil {
		s.slogger.Warn("audit sink: unhandled event type", "topic", ev.Topic())
		return nil
	}
	s.logger.LogBackground(*entry)
	return nil
}

// ── Event → Entry mappers ─────────────────────────────────────────────

// eventToEntry maps a domain event to an audit log Entry.
// Returns nil for unknown event types (should not happen if
// AuditableTopics is kept in sync).
func eventToEntry(ev eventbus.Event) *Entry {
	switch e := ev.(type) {
	case downloadQueued:
		return &Entry{
			Category:   "download",
			EventType:  "download.queued",
			Message:    fmt.Sprintf("Download queued on client %s", e.GetClientID()),
			Detail:     DetailJSON(map[string]any{"download_id": e.GetDownloadID(), "client_id": e.GetClientID(), "origin_result_id": e.GetOriginResultID()}),
			EntityType: StrPtr("download"),
			EntityID:   StrPtr(e.GetDownloadID()),
			Level:      "info",
			Source:     StrPtr("system"),
			OccurredAt: FormatTime(e.GetQueuedAt()),
		}
	case downloadFailed:
		return &Entry{
			Category:   "download",
			EventType:  "download.failed",
			Message:    fmt.Sprintf("Download failed on client %s: %s", e.GetClientID(), truncate(e.GetError(), 200)),
			Detail:     DetailJSON(map[string]any{"client_id": e.GetClientID(), "error": e.GetError(), "origin_result_id": e.GetOriginResultID()}),
			EntityType: StrPtr("download_client"),
			EntityID:   StrPtr(e.GetClientID()),
			Level:      "error",
			Source:     StrPtr("system"),
			OccurredAt: FormatTime(e.GetFailedAt()),
		}
	case downloadCompleted:
		return &Entry{
			Category:   "download",
			EventType:  "download.completed",
			Message:    fmt.Sprintf("Download completed: %s", e.GetTitle()),
			Detail:     DetailJSON(map[string]any{"download_id": e.GetDownloadID(), "client_id": e.GetClientID(), "title": e.GetTitle(), "category": e.GetCategory()}),
			EntityType: StrPtr("download"),
			EntityID:   StrPtr(e.GetDownloadID()),
			EntityName: StrPtr(e.GetTitle()),
			Level:      "info",
			Source:     StrPtr("system"),
			OccurredAt: FormatTime(e.GetCompletedAt()),
		}
	case downloadStalled:
		return &Entry{
			Category:   "download",
			EventType:  "download.stalled",
			Message:    fmt.Sprintf("Download stalled: %s (%s)", e.GetTitle(), e.GetReason()),
			Detail:     DetailJSON(map[string]any{"download_id": e.GetDownloadID(), "title": e.GetTitle(), "reason": e.GetReason(), "action": e.GetAction()}),
			EntityType: StrPtr("download"),
			EntityID:   StrPtr(e.GetDownloadID()),
			EntityName: StrPtr(e.GetTitle()),
			Level:      "warn",
			Source:     StrPtr("system"),
			OccurredAt: FormatTime(e.GetStalledAt()),
		}
	case downloadRetry:
		return &Entry{
			Category:   "download",
			EventType:  "download.retry",
			Message:    fmt.Sprintf("Re-searching after stalled download: %s (attempt %d)", e.GetTitle(), e.GetAttempt()),
			Detail:     DetailJSON(map[string]any{"title": e.GetTitle(), "category": e.GetCategory(), "reason": e.GetReason(), "attempt": e.GetAttempt()}),
			EntityType: StrPtr("download"),
			EntityName: StrPtr(e.GetTitle()),
			Level:      "warn",
			Source:     StrPtr("system"),
			OccurredAt: FormatTime(e.GetRetriedAt()),
		}
	case importCompleted:
		return &Entry{
			Category:   "import",
			EventType:  "import.completed",
			Message:    fmt.Sprintf("Imported: %s", e.GetTitle()),
			Detail:     DetailJSON(map[string]any{"media_type": e.GetMediaType(), "media_id": e.GetMediaID(), "title": e.GetTitle(), "dest_path": e.GetDestPath()}),
			EntityType: StrPtr(e.GetMediaType()),
			EntityID:   StrPtr(e.GetMediaID()),
			EntityName: StrPtr(e.GetTitle()),
			Level:      "info",
			Source:     StrPtr("system"),
		}
	case importFailed:
		return &Entry{
			Category:   "import",
			EventType:  "import.failed",
			Message:    fmt.Sprintf("Import failed: %s — %s", e.GetTitle(), truncate(e.GetError(), 200)),
			Detail:     DetailJSON(map[string]any{"title": e.GetTitle(), "source_path": e.GetSourcePath(), "error": e.GetError()}),
			EntityType: StrPtr("import"),
			EntityName: StrPtr(e.GetTitle()),
			Level:      "error",
			Source:     StrPtr("system"),
		}
	case searchCompleted:
		return &Entry{
			Category:   "search",
			EventType:  "search.completed",
			Message:    fmt.Sprintf("Search completed: %s (%d results)", e.GetTitle(), e.GetResultCount()),
			Detail:     DetailJSON(map[string]any{"media_type": e.GetMediaType(), "media_id": e.GetMediaID(), "title": e.GetTitle(), "result_count": e.GetResultCount()}),
			EntityType: StrPtr(e.GetMediaType()),
			EntityID:   StrPtr(e.GetMediaID()),
			EntityName: StrPtr(e.GetTitle()),
			Level:      "info",
			Source:     StrPtr("system"),
		}
	case searchFailed:
		return &Entry{
			Category:   "search",
			EventType:  "search.failed",
			Message:    fmt.Sprintf("Search failed: %s — %s", e.GetTitle(), truncate(e.GetError(), 200)),
			Detail:     DetailJSON(map[string]any{"media_type": e.GetMediaType(), "media_id": e.GetMediaID(), "title": e.GetTitle(), "error": e.GetError()}),
			EntityType: StrPtr(e.GetMediaType()),
			EntityID:   StrPtr(e.GetMediaID()),
			EntityName: StrPtr(e.GetTitle()),
			Level:      "error",
			Source:     StrPtr("system"),
		}
	case scanImported:
		return &Entry{
			Category:   "scan",
			EventType:  "scan.imported",
			Message:    fmt.Sprintf("File imported: %s (%s)", e.GetTitle(), e.GetQuality()),
			Detail:     DetailJSON(map[string]any{"media_type": e.GetMediaType(), "media_id": e.GetMediaID(), "title": e.GetTitle(), "quality": e.GetQuality(), "file_path": e.GetFilePath()}),
			EntityType: StrPtr(e.GetMediaType()),
			EntityID:   StrPtr(e.GetMediaID()),
			EntityName: StrPtr(e.GetTitle()),
			Level:      "info",
			Source:     StrPtr("system"),
		}
	default:
		return nil
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ── Interfaces for domain events ──────────────────────────────────────
// These use getter interfaces so the sink package doesn't import
// concrete domain event packages (avoiding import cycles).

type downloadQueued interface {
	eventbus.Event
	GetDownloadID() string
	GetClientID() string
	GetOriginResultID() string
	GetQueuedAt() time.Time
}

type downloadFailed interface {
	eventbus.Event
	GetClientID() string
	GetOriginResultID() string
	GetError() string
	GetFailedAt() time.Time
}

type downloadCompleted interface {
	eventbus.Event
	GetDownloadID() string
	GetClientID() string
	GetTitle() string
	GetCategory() string
	GetCompletedAt() time.Time
}

type downloadStalled interface {
	eventbus.Event
	GetDownloadID() string
	GetTitle() string
	GetReason() string
	GetAction() string
	GetStalledAt() time.Time
}

type downloadRetry interface {
	eventbus.Event
	GetTitle() string
	GetCategory() string
	GetReason() string
	GetAttempt() int
	GetRetriedAt() time.Time
}

type importCompleted interface {
	eventbus.Event
	GetMediaType() string
	GetMediaID() string
	GetTitle() string
	GetDestPath() string
}

type importFailed interface {
	eventbus.Event
	GetTitle() string
	GetSourcePath() string
	GetError() string
}

type searchCompleted interface {
	eventbus.Event
	GetMediaType() string
	GetMediaID() string
	GetTitle() string
	GetResultCount() int
}

type searchFailed interface {
	eventbus.Event
	GetMediaType() string
	GetMediaID() string
	GetTitle() string
	GetError() string
}

type scanImported interface {
	eventbus.Event
	GetMediaType() string
	GetMediaID() string
	GetTitle() string
	GetQuality() string
	GetFilePath() string
}
