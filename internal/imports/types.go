package imports

import (
	"time"
)

// ImportMode controls how files are moved from the download directory
// into the media library.
type ImportMode string

const (
	ImportModeMove        ImportMode = "move"
	ImportModeCopy        ImportMode = "copy"
	ImportModeHardlink    ImportMode = "hardlink"
	ImportModeHardlinkOnly ImportMode = "hardlink_only"
)

// ImportStatus represents the outcome of an import attempt.
type ImportStatus string

const (
	StatusPending       ImportStatus = "pending"
	StatusImported      ImportStatus = "imported"
	StatusFailed        ImportStatus = "failed"
	StatusPendingReview ImportStatus = "pending_review"
)

// MediaType distinguishes movies from series episodes.
type MediaType string

const (
	MediaTypeMovie   MediaType = "movie"
	MediaTypeEpisode MediaType = "episode"
)

// ImportRecord is the persisted history of a single file import.
type ImportRecord struct {
	ID         string       `json:"id"`
	MediaType  MediaType    `json:"media_type"`
	MediaID    string       `json:"media_id"`
	SourcePath string       `json:"source_path"`
	DestPath   string       `json:"dest_path"`
	ImportMode ImportMode   `json:"import_mode"`
	Status     ImportStatus `json:"status"`
	Error      string       `json:"error,omitempty"`
	ImportedAt time.Time    `json:"imported_at"`
}

// MatchResult is returned by the matcher when a downloaded file is
// matched to a library item.
type MatchResult struct {
	Matched   bool
	MediaType MediaType
	MediaID   string
	Title     string
	Year      int
	Season    int
	Episode   int
	DestPath  string
}

// TopicImportCompleted is the event bus topic for completed imports.
const TopicImportCompleted = "imports.completed"

// TopicImportFailed is the event bus topic for failed imports.
const TopicImportFailed = "imports.failed"

// ImportCompletedEvent is published when a file is successfully imported.
type ImportCompletedEvent struct {
	MediaType MediaType `json:"media_type"`
	MediaID   string    `json:"media_id"`
	Title     string    `json:"title"`
	DestPath  string    `json:"dest_path"`
}

// Topic implements eventbus.Event.
func (e *ImportCompletedEvent) Topic() string { return TopicImportCompleted }

// ImportFailedEvent is published when an import fails.
type ImportFailedEvent struct {
	Title      string `json:"title"`
	SourcePath string `json:"source_path"`
	Error      string `json:"error"`
}

// Topic implements eventbus.Event.
func (e *ImportFailedEvent) Topic() string { return TopicImportFailed }

// ── Getter methods for audit sink interfaces ──────────────────────────

func (e *ImportCompletedEvent) GetMediaType() string { return string(e.MediaType) }
func (e *ImportCompletedEvent) GetMediaID() string   { return e.MediaID }
func (e *ImportCompletedEvent) GetDestPath() string  { return e.DestPath }

func (e *ImportFailedEvent) GetSourcePath() string { return e.SourcePath }
func (e *ImportFailedEvent) GetError() string      { return e.Error }
