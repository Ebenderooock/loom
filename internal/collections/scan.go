package collections

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ScanStatus exposes durable discovery progress independently of any UI request.
type ScanStatus struct {
	Total   int         `json:"total"`
	Checked int         `json:"checked"`
	Pending int         `json:"pending"`
	Failed  int         `json:"failed"`
	Running bool        `json:"running"`
	Errors  []ScanError `json:"errors"`
}

// ScanError identifies a library title that will be retried automatically.
type ScanError struct {
	MediaType string `json:"media_type"`
	MediaID   string `json:"media_id"`
	Message   string `json:"message"`
}

// ScanStatus reads progress without contacting a provider.
func (s *Service) ScanStatus(ctx context.Context) (*ScanStatus, error) {
	now := time.Now().Unix()
	status := &ScanStatus{Errors: []ScanError{}}
	if err := s.store.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN checked_at>0 THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN next_at<=$1 THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN error<>'' THEN 1 ELSE 0 END),0) FROM collection_scan`, now).
		Scan(&status.Total, &status.Checked, &status.Pending, &status.Failed); err != nil {
		return nil, err
	}
	var lease int64
	if err := s.store.db.QueryRowContext(ctx, `SELECT lease_until FROM collection_scan_lock WHERE id=1`).Scan(&lease); err != nil {
		return nil, err
	}
	status.Running = lease > now
	rows, err := s.store.db.QueryContext(ctx, `SELECT media_type,media_id,error FROM collection_scan WHERE error<>'' ORDER BY position LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var failure ScanError
		if err := rows.Scan(&failure.MediaType, &failure.MediaID, &failure.Message); err != nil {
			return nil, err
		}
		status.Errors = append(status.Errors, failure)
	}
	return status, rows.Err()
}

// RequestScan queues an optional rescan. The scheduler, not the HTTP request,
// performs provider work, including catalog reconciliation for newly added IDs.
func (s *Service) RequestScan(ctx context.Context) (*ScanStatus, error) {
	if _, err := s.store.db.ExecContext(ctx, `UPDATE collection_scan SET next_at=0,checked_at=0`); err != nil {
		return nil, err
	}
	return s.ScanStatus(ctx)
}

// ScanBatch is registered with the persistent scheduler. A database lease
// prevents concurrent workers; per-title checkpoints survive shutdown/restart.
func (s *Service) ScanBatch(ctx context.Context) error {
	return s.scanBatch(ctx, 250*time.Millisecond)
}

func (s *Service) scanBatch(ctx context.Context, pace time.Duration) (result error) {
	if !s.scanMu.TryLock() {
		return nil
	}
	defer s.scanMu.Unlock()
	now := time.Now()
	lease := now.Add(3 * time.Minute).Unix()
	claimed, err := s.store.db.ExecContext(ctx, `UPDATE collection_scan_lock SET lease_until=$1 WHERE id=1 AND lease_until<=$2`, lease, now.Unix())
	if err != nil {
		return err
	}
	count, err := claimed.RowsAffected()
	if err != nil || count == 0 {
		return err
	}
	defer func() {
		release, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, err := s.store.db.ExecContext(release, `UPDATE collection_scan_lock SET lease_until=0 WHERE id=1 AND lease_until=$1`, lease)
		result = errors.Join(result, err)
	}()
	batch, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	library, err := s.library(batch)
	if err != nil {
		return err
	}
	if err := s.queueLibrary(batch, library); err != nil {
		return err
	}
	rows, err := s.store.db.QueryContext(batch, `SELECT media_type,media_id,identity FROM collection_scan WHERE next_at<=$1 ORDER BY checked_at,position LIMIT 16`, now.Unix())
	if err != nil {
		return err
	}
	pending := []Member{}
	for rows.Next() {
		var m Member
		var identity string
		if err := rows.Scan(&m.MediaType, &m.MediaID, &identity); err != nil {
			_ = rows.Close()
			return err
		}
		if err := json.Unmarshal([]byte(identity), &m.ExternalIDs); err != nil {
			_ = rows.Close()
			return err
		}
		pending = append(pending, m)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return err
	}
	var failures []error
	for _, m := range pending {
		if err := batch.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		titleCtx, titleCancel := context.WithTimeout(batch, 15*time.Second)
		_, scanErr := s.discoverMember(titleCtx, m)
		titleCancel()
		message, retry := "", 24*time.Hour
		if scanErr != nil {
			message, retry = scanErr.Error(), 5*time.Minute
			failures = append(failures, fmt.Errorf("%s %s: %w", m.MediaType, m.MediaID, scanErr))
		}
		checked := time.Now()
		if _, err := s.store.db.ExecContext(ctx, `UPDATE collection_scan SET checked_at=$1,next_at=$2,error=$3 WHERE media_type=$4 AND media_id=$5`,
			checked.Unix(), checked.Add(retry).Unix(), message, m.MediaType, m.MediaID); err != nil {
			return errors.Join(append(failures, err)...)
		}
		// In addition to provider-client rate limiting, pace candidate lookups.
		timer := time.NewTimer(pace)
		select {
		case <-batch.Done():
			timer.Stop()
			return errors.Join(append(failures, batch.Err())...)
		case <-timer.C:
		}
	}
	return errors.Join(failures...)
}

func (s *Service) queueLibrary(ctx context.Context, library []libraryRecord) error {
	movies, shows := []Member{}, []Member{}
	for _, record := range library {
		if record.member.MediaType == "movie" {
			movies = append(movies, record.member)
		} else {
			shows = append(shows, record.member)
		}
	}
	// Interleave media so large movie libraries cannot starve shows.
	fair := make([]Member, 0, len(library))
	for i := 0; i < len(movies) || i < len(shows); i++ {
		if i < len(movies) {
			fair = append(fair, movies[i])
		}
		if i < len(shows) {
			fair = append(fair, shows[i])
		}
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Mark existing positions before rebuilding, then remove deleted titles.
	if _, err := tx.ExecContext(ctx, `UPDATE collection_scan SET position=-1`); err != nil {
		return err
	}
	for position, m := range fair {
		identity, err := json.Marshal(m.ExternalIDs)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO collection_scan(media_type,media_id,identity,position) VALUES($1,$2,$3,$4)
			ON CONFLICT(media_type,media_id) DO UPDATE SET position=excluded.position,
			next_at=CASE WHEN collection_scan.identity<>excluded.identity THEN 0 ELSE collection_scan.next_at END,
			checked_at=CASE WHEN collection_scan.identity<>excluded.identity THEN 0 ELSE collection_scan.checked_at END,
			identity=excluded.identity`, m.MediaType, m.MediaID, string(identity), position)
		if err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM collection_scan WHERE position=-1`); err != nil {
		return err
	}
	return tx.Commit()
}
