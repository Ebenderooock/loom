package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ebenderooock/loom/internal/auth"
)

func (h *HistoryStore) acquisitionAction(ctx context.Context, id, action string) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var key, title, indexer, hash, status, admittedAt string
	err = tx.QueryRowContext(ctx, `SELECT acquisition_key, title, indexer_id, release_hash, status, completed_at
		FROM download_history WHERE id = ? AND acquisition_key <> ''
		AND attempt = (SELECT MAX(d.attempt) FROM download_history d WHERE d.acquisition_key = download_history.acquisition_key)`, id).
		Scan(&key, &title, &indexer, &hash, &status, &admittedAt)
	if err != nil {
		return err
	}
	if status != "acquisition_failed" && status != "acquiring" {
		return ErrAcquisitionDeferred
	}
	if status == "acquiring" {
		at, parseErr := time.Parse(time.RFC3339, admittedAt)
		if parseErr != nil {
			return parseErr
		}
		if time.Since(at) < 15*time.Minute {
			return ErrAcquisitionDeferred
		}
	}
	switch action {
	case "retry":
		// Re-arm eligibility, not an old signed URL. The next acquisition must
		// obtain fresh release coordinates through its normal search flow.
		_, err = tx.ExecContext(ctx, `UPDATE download_history SET status = 'acquisition_failed',
			retry_eligible = 1, retry_count = 0, next_retry_at = '', reason = 'Retry authorized by administrator.'
			WHERE id = ?`, id)
	case "blocklist":
		if strings.HasPrefix(key, "search:") {
			return ErrAcquisitionDeferred
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO blocklist (id, title, indexer_id, release_hash, reason, created_at)
			VALUES (?, ?, ?, ?, 'Acquisition blocked by administrator.', ?)
			ON CONFLICT(id) DO NOTHING`, key, title, indexer, hash, time.Now().UTC())
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE download_history SET status = 'acquisition_failed',
				retry_eligible = 0, next_retry_at = '', reason = 'Release blocklisted by administrator.'
				WHERE id = ?`, id)
		}
	default:
		return errors.New("unknown acquisition action")
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) handleAcquisitionActions(w http.ResponseWriter, r *http.Request) {
	identity := auth.IdentityFrom(r.Context())
	if identity == nil || !identity.HasRole("admin") {
		writeError(w, http.StatusForbidden, "forbidden", "admin access required")
		return
	}
	var req struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil ||
		len(req.IDs) == 0 || len(req.IDs) > 100 || (req.Action != "retry" && req.Action != "blocklist") {
		writeError(w, http.StatusBadRequest, "invalid_body", "provide 1-100 history IDs and retry or blocklist action")
		return
	}
	if s.historyStore == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "download history unavailable")
		return
	}
	type outcome struct {
		ID    string `json:"id"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	results := make([]outcome, 0, len(req.IDs))
	for _, id := range req.IDs {
		result := outcome{ID: id}
		err := s.historyStore.acquisitionAction(r.Context(), id, req.Action)
		switch {
		case err == nil:
			result.OK = true
		case errors.Is(err, sql.ErrNoRows):
			result.Error = "History item not found or superseded."
		case errors.Is(err, ErrAcquisitionDeferred):
			result.Error = "Item is successful, still in its 15-minute admission window, or has no release to blocklist."
		default:
			s.logger.Error("acquisition action failed", "id", id, "error", err)
			result.Error = "Unable to persist acquisition action."
		}
		results = append(results, result)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
