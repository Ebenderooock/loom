package imports

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// SmartReimportOptions extends reimport with multi-criteria comparison.
type SmartReimportOptions struct {
	ForceReplace bool `json:"force_replace"`
}

// ReplacementRecord tracks the history of file replacements.
type ReplacementRecord struct {
	ID         string `json:"id"`
	MediaType  string `json:"media_type"`
	MediaID    string `json:"media_id"`
	OldPath    string `json:"old_path"`
	NewPath    string `json:"new_path"`
	OldQuality string `json:"old_quality"`
	NewQuality string `json:"new_quality"`
	OldSize    int64  `json:"old_size"`
	NewSize    int64  `json:"new_size"`
	OldScore   int    `json:"old_score"`
	NewScore   int    `json:"new_score"`
	Reason     string `json:"reason"`
	ReplacedAt string `json:"replaced_at"`
}

// ReplacementStore manages replacement history records.
type ReplacementStore struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewReplacementStore creates a ReplacementStore.
func NewReplacementStore(db *sql.DB, logger *slog.Logger) *ReplacementStore {
	return &ReplacementStore{db: db, logger: logger}
}

// Record persists a replacement event.
func (rs *ReplacementStore) Record(ctx context.Context, rec ReplacementRecord) error {
	if rec.ID == "" {
		rec.ID = uuid.New().String()
	}
	if rec.ReplacedAt == "" {
		rec.ReplacedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := rs.db.ExecContext(ctx,
		`INSERT INTO replacement_history (id, media_type, media_id, old_path, new_path, old_quality, new_quality, old_size, new_size, old_score, new_score, reason, replaced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.MediaType, rec.MediaID, rec.OldPath, rec.NewPath,
		rec.OldQuality, rec.NewQuality, rec.OldSize, rec.NewSize,
		rec.OldScore, rec.NewScore, rec.Reason, rec.ReplacedAt,
	)
	return err
}

// ListByMedia returns replacement history for a specific media item.
func (rs *ReplacementStore) ListByMedia(ctx context.Context, mediaType, mediaID string, limit int) ([]ReplacementRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := rs.db.QueryContext(ctx,
		`SELECT id, media_type, media_id, old_path, new_path, old_quality, new_quality, old_size, new_size, old_score, new_score, reason, replaced_at
		 FROM replacement_history
		 WHERE media_type = ? AND media_id = ?
		 ORDER BY replaced_at DESC
		 LIMIT ?`,
		mediaType, mediaID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ReplacementRecord
	for rows.Next() {
		var r ReplacementRecord
		if err := rows.Scan(
			&r.ID, &r.MediaType, &r.MediaID, &r.OldPath, &r.NewPath,
			&r.OldQuality, &r.NewQuality, &r.OldSize, &r.NewSize,
			&r.OldScore, &r.NewScore, &r.Reason, &r.ReplacedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// ShouldUpgrade compares quality, custom format score, and file size.
// Returns true only if the new file scores higher across ALL criteria,
// unless ForceReplace is set.
func ShouldUpgrade(existing, incoming FileInfo, existingCFScore, incomingCFScore int, opts SmartReimportOptions) (bool, string) {
	if opts.ForceReplace {
		return true, "force_replace requested (corrupted file fix)"
	}

	existingQS := qualityScore(existing.Quality)
	incomingQS := qualityScore(incoming.Quality)

	// Must be better or equal on all criteria, strictly better on at least one
	qualityBetter := incomingQS >= existingQS
	scoreBetter := incomingCFScore >= existingCFScore
	sizeBetter := incoming.Size >= existing.Size

	if !qualityBetter {
		return false, fmt.Sprintf("incoming quality %d < existing %d", incomingQS, existingQS)
	}
	if !scoreBetter {
		return false, fmt.Sprintf("incoming CF score %d < existing %d", incomingCFScore, existingCFScore)
	}

	strictlyBetter := incomingQS > existingQS || incomingCFScore > existingCFScore || (sizeBetter && incoming.Size > existing.Size)
	if !strictlyBetter {
		return false, "incoming file is not strictly better on any criterion"
	}

	return true, fmt.Sprintf("upgrade: quality %d→%d, cf_score %d→%d, size %d→%d",
		existingQS, incomingQS, existingCFScore, incomingCFScore, existing.Size, incoming.Size)
}
