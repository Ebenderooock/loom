package safety

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ReviewStatus represents the state of a manual review entry.
type ReviewStatus string

const (
	ReviewPending  ReviewStatus = "pending"
	ReviewApproved ReviewStatus = "approved"
	ReviewRejected ReviewStatus = "rejected"
)

// Review is a manual-intervention record created when a release passes
// with severity "warning".
type Review struct {
	ID           string       `json:"id"`
	MediaType    string       `json:"media_type"`
	MediaID      string       `json:"media_id"`
	DownloadPath string       `json:"download_path"`
	Reason       string       `json:"reason"`
	Status       ReviewStatus `json:"status"`
	CreatedAt    string       `json:"created_at"`
	ResolvedAt   *string      `json:"resolved_at,omitempty"`
}

// ReviewStore provides persistence for manual-review entries.
type ReviewStore struct {
	db *sql.DB
}

// NewReviewStore wraps a *sql.DB for review CRUD.
func NewReviewStore(db *sql.DB) *ReviewStore {
	return &ReviewStore{db: db}
}

// Create inserts a new pending review.
func (s *ReviewStore) Create(ctx context.Context, mediaType, mediaID, downloadPath, reason string) (*Review, error) {
	r := &Review{
		ID:           uuid.New().String(),
		MediaType:    mediaType,
		MediaID:      mediaID,
		DownloadPath: downloadPath,
		Reason:       reason,
		Status:       ReviewPending,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO manual_review (id, media_type, media_id, download_path, reason, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.ID, r.MediaType, r.MediaID, r.DownloadPath, r.Reason, string(r.Status),
	)
	if err != nil {
		return nil, fmt.Errorf("create review: %w", err)
	}
	return r, nil
}

// ListPending returns all reviews with status "pending".
func (s *ReviewStore) ListPending(ctx context.Context) ([]*Review, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, media_type, media_id, download_path, reason, status, created_at, resolved_at
		 FROM manual_review WHERE status = ? ORDER BY created_at DESC`, string(ReviewPending))
	if err != nil {
		return nil, fmt.Errorf("list pending reviews: %w", err)
	}
	defer rows.Close()
	return scanReviews(rows)
}

// CountPending returns the number of pending reviews.
func (s *ReviewStore) CountPending(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM manual_review WHERE status = ?`, string(ReviewPending)).Scan(&count)
	return count, err
}

// Approve marks a review as approved.
func (s *ReviewStore) Approve(ctx context.Context, id string) error {
	return s.resolve(ctx, id, ReviewApproved)
}

// Reject marks a review as rejected.
func (s *ReviewStore) Reject(ctx context.Context, id string) error {
	return s.resolve(ctx, id, ReviewRejected)
}

func (s *ReviewStore) resolve(ctx context.Context, id string, status ReviewStatus) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE manual_review SET status = ?, resolved_at = ? WHERE id = ? AND status = ?`,
		string(status), now, id, string(ReviewPending),
	)
	if err != nil {
		return fmt.Errorf("resolve review: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("review %q not found or already resolved", id)
	}
	return nil
}

func scanReviews(rows *sql.Rows) ([]*Review, error) {
	var out []*Review
	for rows.Next() {
		r := &Review{}
		if err := rows.Scan(&r.ID, &r.MediaType, &r.MediaID, &r.DownloadPath, &r.Reason, &r.Status, &r.CreatedAt, &r.ResolvedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ─── HTTP handlers ──────────────────────────────────────────────────────

// Router returns a chi.Router with manual-review endpoints.
func Router(store *ReviewStore) chi.Router {
	r := chi.NewRouter()
	r.Get("/", listReviews(store))
	r.Get("/count", countReviews(store))
	r.Post("/{id}/approve", approveReview(store))
	r.Post("/{id}/reject", rejectReview(store))
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    http.StatusText(status),
			"message": msg,
		},
	})
}

func listReviews(store *ReviewStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reviews, err := store.ListPending(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if reviews == nil {
			reviews = []*Review{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": reviews})
	}
}

func countReviews(store *ReviewStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		count, err := store.CountPending(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"count": count})
	}
}

func approveReview(store *ReviewStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := store.Approve(r.Context(), id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "approved"})
	}
}

func rejectReview(store *ReviewStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := store.Reject(r.Context(), id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "rejected"})
	}
}
