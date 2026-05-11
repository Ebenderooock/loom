package workflows

import "time"

// Workflow states
const (
	StateSearching   = "searching"
	StateGrabbed     = "grabbed"
	StateDownloading = "downloading"
	StateImporting   = "importing"
	StateCompleted   = "completed"
	StateFailed      = "failed"
	StateCancelled   = "cancelled"
)

// Workflow types
const (
	TypeMovieSearch   = "movie_search"
	TypeEpisodeSearch = "episode_search"
	TypeManualImport  = "manual_import"
)

// Media types
const (
	MediaTypeMovie   = "movie"
	MediaTypeEpisode = "episode"
)

// Item states (per-media-item within a workflow)
const (
	ItemPending = "pending"
)

// State timeouts for stale detection
var StateTimeouts = map[string]time.Duration{
	StateSearching:   5 * time.Minute,
	StateGrabbed:     10 * time.Minute,
	StateDownloading: 24 * time.Hour,
	StateImporting:   30 * time.Minute,
}

// Valid transitions from each state
var ValidTransitions = map[string][]string{
	StateSearching:   {StateGrabbed, StateFailed, StateCancelled},
	StateGrabbed:     {StateDownloading, StateFailed, StateCancelled},
	StateDownloading: {StateImporting, StateFailed, StateCancelled},
	StateImporting:   {StateCompleted, StateFailed, StateCancelled},
	StateFailed:      {StateSearching, StateImporting}, // retry paths
}

// Retry behavior per failed state
const (
	MaxRetries     = 3
	RetryBackoff1  = 5 * time.Minute
	RetryBackoff2  = 15 * time.Minute
	RetryBackoff3  = 45 * time.Minute
	CompletedTTL   = 7 * 24 * time.Hour // prune completed after 7 days
	SchedulerTick  = 60 * time.Second
)

func RetryBackoff(attempt int) time.Duration {
	switch attempt {
	case 1:
		return RetryBackoff1
	case 2:
		return RetryBackoff2
	default:
		return RetryBackoff3
	}
}

// Workflow represents a media acquisition pipeline.
type Workflow struct {
	ID               string     `json:"id"`
	Type             string     `json:"type"`
	State            string     `json:"state"`
	MediaType        string     `json:"mediaType"`
	GrabTitle        string     `json:"grabTitle,omitempty"`
	DownloadClientID string     `json:"downloadClientId,omitempty"`
	DownloadID       string     `json:"downloadId,omitempty"`
	QualityProfileID string     `json:"qualityProfileId,omitempty"`
	RetryCount       int        `json:"retryCount"`
	MaxRetries       int        `json:"maxRetries"`
	LastError        string     `json:"lastError,omitempty"`
	Metadata         string     `json:"metadata,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
	Items            []Item     `json:"items,omitempty"`
	History          []Event    `json:"history,omitempty"`
}

// Item represents a media item claimed by a workflow (junction only, no per-item state).
type Item struct {
	WorkflowID string `json:"workflowId"`
	MediaType  string `json:"mediaType"`
	MediaID    string `json:"mediaId"`
}

// Event is a workflow state transition record.
type Event struct {
	ID         int64     `json:"id"`
	WorkflowID string    `json:"workflowId"`
	FromState  string    `json:"fromState"`
	ToState    string    `json:"toState"`
	Message    string    `json:"message,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// IsTerminal returns true if the workflow is in a final state.
func (w *Workflow) IsTerminal() bool {
	return w.State == StateCompleted || w.State == StateFailed || w.State == StateCancelled
}

// IsActive returns true if the workflow is in a non-terminal state.
func (w *Workflow) IsActive() bool {
	return !w.IsTerminal()
}

// CanTransitionTo checks if a state transition is valid.
func CanTransitionTo(from, to string) bool {
	allowed, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
