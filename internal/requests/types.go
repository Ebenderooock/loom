// Package requests implements the media-requests feature (Overseerr-equivalent):
// authenticated users submit requests for movies or series, and admins approve
// or reject them. Approval adds the media (monitored) and triggers a
// search-and-grab. All descriptive metadata is fetched server-side from the
// trusted provider on approval — caller-supplied fields are used only for
// display and audit, never for fulfillment.
package requests

import "time"

// Status is the lifecycle state of a media request.
type Status string

const (
	// StatusPending — submitted, awaiting an admin decision.
	StatusPending Status = "pending"
	// StatusApproving — an admin accepted it and fulfillment is in progress.
	// Acts as a lock so a request is only fulfilled once.
	StatusApproving Status = "approving"
	// StatusApproved — media was added to the library (grab runs async).
	StatusApproved Status = "approved"
	// StatusRejected — an admin declined the request.
	StatusRejected Status = "rejected"
	// StatusFailed — fulfillment (add) errored; the request is re-requestable.
	StatusFailed Status = "failed"
	// StatusAvailable — reserved for a future "downloaded & imported" transition.
	StatusAvailable Status = "available"
)

// MediaType distinguishes movie vs series requests.
type MediaType string

const (
	MediaMovie  MediaType = "movie"
	MediaSeries MediaType = "series"
)

// Request is a single user request for a piece of media.
type Request struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Username   string     `json:"username"`
	MediaType  MediaType  `json:"media_type"`
	TMDBID     string     `json:"tmdb_id"`
	Title      string     `json:"title"`
	Year       int        `json:"year"`
	PosterPath string     `json:"poster_path,omitempty"`
	Overview   string     `json:"overview,omitempty"`
	Status     Status     `json:"status"`
	Reason     string     `json:"reason,omitempty"`
	MediaID    string     `json:"media_id,omitempty"`
	DecidedBy  string     `json:"decided_by,omitempty"`
	DecidedAt  *time.Time `json:"decided_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateInput is the data needed to submit a request. Title/Year/PosterPath/
// Overview are display-only hints captured from the lookup at request time.
type CreateInput struct {
	MediaType  MediaType `json:"media_type"`
	TMDBID     string    `json:"tmdb_id"`
	Title      string    `json:"title"`
	Year       int       `json:"year"`
	PosterPath string    `json:"poster_path"`
	Overview   string    `json:"overview"`
}

// validMediaType reports whether t is a supported media type.
func validMediaType(t MediaType) bool {
	return t == MediaMovie || t == MediaSeries
}
