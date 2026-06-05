package movies

import "time"

// Event topics for the movies service. These are published to the event bus
// for downstream consumers (e.g., search indexing, dashboard updates).
const (
	TopicMovieAdded         = "movies.added"
	TopicMovieUpdated       = "movies.updated"
	TopicMovieDeleted       = "movies.deleted"
	TopicMonitoringChanged  = "movies.monitoring_changed"
	TopicMovieStatusChanged = "movies.status_changed"
)

// MovieStatusChangedEvent is published when a movie's status transitions
// (e.g. missing → downloading → available). The audit sink and frontend
// SSE consumers use this to keep the UI in sync.
type MovieStatusChangedEvent struct {
	MovieID   string      `json:"movie_id"`
	Title     string      `json:"title"`
	OldStatus MovieStatus `json:"old_status"`
	NewStatus MovieStatus `json:"new_status"`
	ChangedAt time.Time   `json:"changed_at"`
}

func (e *MovieStatusChangedEvent) Topic() string { return TopicMovieStatusChanged }

// Getter methods for the audit sink (interface-based matching avoids import cycles).
func (e *MovieStatusChangedEvent) GetMovieID() string      { return e.MovieID }
func (e *MovieStatusChangedEvent) GetTitle() string        { return e.Title }
func (e *MovieStatusChangedEvent) GetOldStatus() string    { return string(e.OldStatus) }
func (e *MovieStatusChangedEvent) GetNewStatus() string    { return string(e.NewStatus) }
func (e *MovieStatusChangedEvent) GetChangedAt() time.Time { return e.ChangedAt }
