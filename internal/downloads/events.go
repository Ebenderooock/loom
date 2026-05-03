package downloads

import (
	"fmt"
	"time"

	"github.com/loomctl/loom/internal/indexers"
)

// Event topics for the downloads service. These are published to the
// event bus for downstream consumers (e.g., releases marking items as
// acquired when a download completes).
const (
	TopicDownloadQueued     = "downloads.queued"
	TopicDownloadFailed     = "downloads.failed"
	TopicDownloadCompleted  = "downloads.completed"
	TopicIndexerResult      = "indexers.result" // Expected from indexers; we listen for this
)

// IndexerResultEvent wraps an indexers.Result for the event bus. It
// implements the Event interface so it can be published/subscribed.
type IndexerResultEvent struct {
	Result *indexers.Result
}

// Topic implements the Event interface.
func (e *IndexerResultEvent) Topic() string { return TopicIndexerResult }

// DownloadQueuedEvent is fired when a Result is successfully queued on
// a download client. Downstream consumers use this to track what was
// attempted and succeed early if no quality rules apply.
type DownloadQueuedEvent struct {
	// DownloadID is the per-client opaque identifier (e.g. infohash
	// for torrents, NZB ID for usenet).
	DownloadID string `json:"download_id"`

	// OriginResultID is the indexer Result GUID that led to this
	// download, for traceability across the intake pipeline.
	OriginResultID string `json:"origin_result_id"`

	// ClientID is the configured download client this was queued on.
	ClientID string `json:"client_id"`

	// QueuedAt is when this event was emitted.
	QueuedAt time.Time `json:"queued_at"`
}

// Topic implements the Event interface.
func (e *DownloadQueuedEvent) Topic() string { return TopicDownloadQueued }

// DownloadFailureEvent is fired when Add() fails for any reason.
// Downstream consumers may retry or escalate depending on the reason.
type DownloadFailureEvent struct {
	// OriginResultID is the indexer Result GUID that led to this
	// attempt.
	OriginResultID string `json:"origin_result_id"`

	// ClientID is the download client that rejected the attempt.
	ClientID string `json:"client_id"`

	// Error is the reason Add() failed (e.g. client offline, disk full).
	Error string `json:"error"`

	// FailedAt is when this event was emitted.
	FailedAt time.Time `json:"failed_at"`
}

// Topic implements the Event interface.
func (e *DownloadFailureEvent) Topic() string { return TopicDownloadFailed }

// DownloadCompletedEvent is fired when the monitor detects that a
// download has finished. Downstream consumers (releases) mark the
// item as acquired.
type DownloadCompletedEvent struct {
	// DownloadID is the per-client identifier that completed.
	DownloadID string `json:"download_id"`

	// ClientID is which client reported the completion.
	ClientID string `json:"client_id"`

	// Title is the human-readable name of the completed item.
	Title string `json:"title"`

	// Category is the grouping the item was filed under.
	Category string `json:"category,omitempty"`

	// CompletedAt is when this event was emitted (not when the item
	// actually finished; see Item.CompletedAt if per-client tracking
	// is needed).
	CompletedAt time.Time `json:"completed_at"`
}

// Topic implements the Event interface.
func (e *DownloadCompletedEvent) Topic() string { return TopicDownloadCompleted }

// String returns a human-readable summary of the event.
func (e *DownloadQueuedEvent) String() string {
	return fmt.Sprintf("DownloadQueuedEvent{DownloadID=%s, ClientID=%s, QueuedAt=%s}",
		e.DownloadID, e.ClientID, e.QueuedAt.Format(time.RFC3339))
}

// String returns a human-readable summary of the event.
func (e *DownloadFailureEvent) String() string {
	return fmt.Sprintf("DownloadFailureEvent{ClientID=%s, Error=%q, FailedAt=%s}",
		e.ClientID, e.Error, e.FailedAt.Format(time.RFC3339))
}

// String returns a human-readable summary of the event.
func (e *DownloadCompletedEvent) String() string {
	return fmt.Sprintf("DownloadCompletedEvent{DownloadID=%s, ClientID=%s, CompletedAt=%s}",
		e.DownloadID, e.ClientID, e.CompletedAt.Format(time.RFC3339))
}
