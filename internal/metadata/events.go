package metadata

import (
	"fmt"
	"time"

	"github.com/ebenderooock/loom/internal/indexers"
)

// Event topics for metadata enrichment pipeline. These are published to the
// event bus for downstream consumers (e.g., databases, search indexes).
const (
	TopicMetadataEnriched = "metadata.enriched"  // Emitted after successful lookup
	TopicMetadataFailure  = "metadata.failure"   // Emitted after timeout/failure
)

// MetadataEnrichedEvent is fired when a download result is successfully
// enriched with movie or series metadata. It wraps the original indexer
// Result plus matched metadata for downstream consumption.
type MetadataEnrichedEvent struct {
	// OriginResultID is the indexer Result GUID for traceability.
	OriginResultID string `json:"origin_result_id"`

	// DownloadID is the per-client download identifier (if the item was queued).
	DownloadID string `json:"download_id,omitempty"`

	// Title is the human-readable name from the indexer result.
	Title string `json:"title"`

	// MovieMetadata is set if this is a movie result.
	MovieMetadata *MovieMetadata `json:"movie_metadata,omitempty"`

	// SeriesMetadata is set if this is a series result.
	SeriesMetadata *SeriesMetadata `json:"series_metadata,omitempty"`

	// EpisodeMetadata is set if this is an episode result.
	EpisodeMetadata *EpisodeMetadata `json:"episode_metadata,omitempty"`

	// EnrichedAt is when this event was emitted.
	EnrichedAt time.Time `json:"enriched_at"`

	// SourceProvider is the metadata provider that matched this result.
	SourceProvider string `json:"source_provider"`
}

// Topic implements the Event interface.
func (e *MetadataEnrichedEvent) Topic() string { return TopicMetadataEnriched }

// String returns a human-readable summary of the event.
func (e *MetadataEnrichedEvent) String() string {
	return fmt.Sprintf("MetadataEnrichedEvent{OriginResultID=%s, Title=%s, SourceProvider=%s, EnrichedAt=%s}",
		e.OriginResultID, e.Title, e.SourceProvider, e.EnrichedAt.Format(time.RFC3339))
}

// MetadataFailureEvent is fired when metadata enrichment fails due to timeout
// or all providers returning no results. Downstream consumers can use this to
// track partial acquisitions (downloaded but unenhanced).
type MetadataFailureEvent struct {
	// OriginResultID is the indexer Result GUID for traceability.
	OriginResultID string `json:"origin_result_id"`

	// DownloadID is the per-client download identifier (if the item was queued).
	DownloadID string `json:"download_id,omitempty"`

	// Title is the human-readable name from the indexer result.
	Title string `json:"title"`

	// Reason describes why enrichment failed (e.g. "timeout", "no match", "all providers failed").
	Reason string `json:"reason"`

	// FailedAt is when this event was emitted.
	FailedAt time.Time `json:"failed_at"`
}

// Topic implements the Event interface.
func (e *MetadataFailureEvent) Topic() string { return TopicMetadataFailure }

// String returns a human-readable summary of the event.
func (e *MetadataFailureEvent) String() string {
	return fmt.Sprintf("MetadataFailureEvent{OriginResultID=%s, Title=%s, Reason=%q, FailedAt=%s}",
		e.OriginResultID, e.Title, e.Reason, e.FailedAt.Format(time.RFC3339))
}

// IndexerResultWithMetadata carries the enrichment context: the original
// indexer Result plus any metadata that was successfully matched.
type IndexerResultWithMetadata struct {
	Result             *indexers.Result
	MovieMetadata      *MovieMetadata
	SeriesMetadata     *SeriesMetadata
	EpisodeMetadata    *EpisodeMetadata
	SourceProvider     string
	EnrichmentErr      error
}
