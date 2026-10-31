package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const availabilityFreshness = 5 * time.Minute
const storageMarker = ".loom-request-storage"

// ReconciliationOutcome records one observation independently of fulfillment.
type ReconciliationOutcome struct {
	RequestID string       `json:"request_id"`
	Before    Availability `json:"before"`
	After     Availability `json:"after"`
	Reason    string       `json:"reason"`
	Changed   bool         `json:"changed"`
	Error     string       `json:"error,omitempty"`
}

// ReconciliationRun is the persisted audit for a preview or applied batch.
type ReconciliationRun struct {
	ID        string                  `json:"id"`
	Actor     string                  `json:"actor"`
	DryRun    bool                    `json:"dry_run"`
	CreatedAt time.Time               `json:"created_at"`
	Outcomes  []ReconciliationOutcome `json:"outcomes"`
}

// ReconcileAvailability refreshes current presence without undoing fulfillment.
func (s *Store) ReconcileAvailability(ctx context.Context, now time.Time) error {
	_, err := s.Reconcile(ctx, now, "scheduler", false)
	return err
}

// Reconcile observes files, applies non-destructive request-only changes, and
// commits the batch with its audit. Dry runs never change requests or storage.
func (s *Store) Reconcile(ctx context.Context, now time.Time, actor string, dryRun bool) (ReconciliationRun, error) {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	all, err := s.query(ctx, `SELECT `+selectCols+` FROM media_requests WHERE status IN ('approved','available') ORDER BY id`)
	if err != nil {
		return ReconciliationRun{}, err
	}
	run := ReconciliationRun{ID: uuid.NewString(), Actor: actor, DryRun: dryRun, CreatedAt: now, Outcomes: []ReconciliationOutcome{}}
	libraryIDs := make([]string, len(all))
	for i, r := range all {
		if err := ctx.Err(); err != nil {
			return ReconciliationRun{}, err
		}
		after, reason, libraryID, observeErr := s.observe(ctx, &r, now, !dryRun)
		all[i].AvailabilityFiles = r.AvailabilityFiles
		libraryIDs[i] = libraryID
		o := ReconciliationOutcome{RequestID: r.ID, Before: r.Availability, After: after, Reason: reason}
		o.Changed = r.Availability != after || r.AvailabilityReason != reason ||
			(r.Status == StatusApproved && after == AvailabilityPresent)
		if observeErr != nil {
			o.Error = observeErr.Error()
		}
		run.Outcomes = append(run.Outcomes, o)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReconciliationRun{}, err
	}
	defer func() { _ = tx.Rollback() }()
	audit := actor != "scheduler"
	for i, o := range run.Outcomes {
		audit = audit || o.Changed || o.Error != ""
		if dryRun {
			continue
		}
		r := all[i]
		status := r.Status
		var fulfilled any
		if r.FulfilledAt != nil {
			fulfilled = r.FulfilledAt.UTC().Format(tsLayout)
		}
		if status == StatusApproved && o.After == AvailabilityPresent {
			status = StatusAvailable
			fulfilled = now.UTC().Format(tsLayout)
		}
		res, err := tx.ExecContext(ctx, `UPDATE media_requests SET status=?, fulfilled_at=?,
			availability=?, availability_reason=?, availability_checked_at=?, availability_library_id=?, availability_files=?,
			updated_at=CASE WHEN ? THEN ? ELSE updated_at END
			WHERE id=? AND status=? AND media_id=?`,
			status, fulfilled, o.After, o.Reason, now.UTC().Format(tsLayout), libraryIDs[i], r.AvailabilityFiles,
			o.Changed, now.UTC().Format(tsLayout), r.ID, r.Status, r.MediaID)
		if err := affectedOne(res, err); err != nil {
			return ReconciliationRun{}, err
		}
	}
	if audit {
		payload, err := json.Marshal(run.Outcomes)
		if err != nil {
			return ReconciliationRun{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO request_reconciliations(id,actor,dry_run,created_at,outcomes)
			VALUES(?,?,?,?,?)`, run.ID, actor, dryRun, now.UTC().Format(tsLayout), string(payload)); err != nil {
			return ReconciliationRun{}, err
		}
	}
	return run, tx.Commit()
}

// ReconciliationHistory returns the latest 50 batches, including dry runs.
func (s *Store) ReconciliationHistory(ctx context.Context) ([]ReconciliationRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,actor,dry_run,created_at,outcomes
		FROM request_reconciliations ORDER BY created_at DESC,id DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ReconciliationRun{}
	for rows.Next() {
		var r ReconciliationRun
		var created, payload string
		if err := rows.Scan(&r.ID, &r.Actor, &r.DryRun, &created, &payload); err != nil {
			return nil, err
		}
		r.CreatedAt = parseTS(created)
		if err := json.Unmarshal([]byte(payload), &r.Outcomes); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func mediaTable(mt MediaType) string {
	switch mt {
	case MediaMovie:
		return "movies"
	case MediaArtist:
		return "artists"
	default:
		return "series"
	}
}

func (s *Store) loadMonitoring(ctx context.Context, r *Request) error {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT monitoring_status FROM `+mediaTable(r.MediaType)+` WHERE id=?`, r.MediaID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	monitored := status == "monitored" || status == "all"
	r.Monitored = &monitored
	return nil
}

// fileGroups returns alternate files per required media item. SQL completeness
// handles absent metadata and required items with no recorded files.
func (s *Store) fileGroups(ctx context.Context, r Request, now time.Time) (map[string][]string, bool, error) {
	var complete bool
	err := s.db.QueryRowContext(ctx, `SELECT `+completionPredicate+` FROM media_requests WHERE id=?`,
		now.Format(time.RFC3339), now.Format(time.DateOnly), now.Format(time.DateOnly), r.ID).Scan(&complete)
	if err != nil {
		return nil, false, err
	}
	q := `SELECT movie_id,file_path FROM movie_files WHERE movie_id=? AND deleted_at IS NULL`
	args := []any{r.MediaID}
	switch r.MediaType {
	case MediaSeries:
		q = `SELECT e.id,COALESCE(f.file_path,'') FROM episodes e
			JOIN seasons sn ON sn.id=e.season_id LEFT JOIN episode_files f ON f.episode_id=e.id
			WHERE e.series_id=? AND e.monitored=true AND sn.monitored=true AND sn.season_number<>0 AND (
				(e.air_date_utc<>'' AND e.air_date_utc<=?) OR
				(e.air_date_utc='' AND (e.air_date IS NULL OR e.air_date='' OR e.air_date<=?)))`
		args = append(args, now.Format(time.RFC3339), now.Format(time.DateOnly))
	case MediaArtist:
		q = `SELECT t.id,COALESCE(f.file_path,'') FROM tracks t
			JOIN albums al ON al.id=t.album_id LEFT JOIN track_files f ON f.track_id=t.id AND f.deleted_at IS NULL
			WHERE al.artist_id=? AND al.deleted_at IS NULL
			AND al.monitored=true AND t.monitored=true AND t.release_id=al.selected_release_id
			AND (al.release_date='' OR al.release_date<=?)`
		args = append(args, now.Format(time.DateOnly))
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	groups := map[string][]string{}
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, false, err
		}
		if path != "" {
			groups[id] = append(groups[id], path)
		} else if _, exists := groups[id]; !exists {
			groups[id] = nil
		}
	}
	return groups, complete, rows.Err()
}

func (s *Store) observe(ctx context.Context, r *Request, now time.Time, establish bool) (Availability, string, string, error) {
	var libraryID string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(library_id,'') FROM `+mediaTable(r.MediaType)+` WHERE id=?`, r.MediaID).Scan(&libraryID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return AvailabilityUnknown, "Cannot read media", "", err
	}
	if libraryID == "" {
		libraryID = r.AvailabilityLibraryID
	}
	var root string
	err = s.db.QueryRowContext(ctx, `SELECT path FROM libraries WHERE id=?`, libraryID).Scan(&root)
	if errors.Is(err, sql.ErrNoRows) {
		return AvailabilityUnknown, "No storage root linked; cannot confirm deletion", libraryID, nil
	}
	if err != nil {
		return AvailabilityUnknown, "Cannot read library", libraryID, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return AvailabilityUnknown, "Storage root is inaccessible", libraryID, err
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return AvailabilityUnknown, "Storage root is not an accessible directory", libraryID, err
	}
	groups, complete, err := s.fileGroups(ctx, *r, now)
	if err != nil {
		return AvailabilityUnknown, "Cannot read file evidence", libraryID, err
	}
	// Retain last observed paths if catalog/file rows were removed. Without
	// path evidence an incomplete or stale scan is not confirmed deletion.
	previous := map[string][]string{}
	if r.AvailabilityFiles != "" {
		if err := json.Unmarshal([]byte(r.AvailabilityFiles), &previous); err != nil {
			return AvailabilityUnknown, "Cannot read prior file evidence", libraryID, err
		}
	}
	if len(groups) == 0 {
		groups = previous
	} else {
		for id, paths := range groups {
			if len(paths) == 0 {
				groups[id] = previous[id]
			}
		}
	}
	allPresent, anyPresent, knownMissing := len(groups) > 0, false, false
	verified := map[string][]string{}
	for id, paths := range groups {
		present := false
		missingVerified := false
		for _, path := range paths {
			resolved, exists, err := resolveEvidencePath(path)
			if err != nil {
				return AvailabilityUnknown, "File cannot be inspected", libraryID, err
			}
			if !pathInside(root, resolved) {
				return AvailabilityUnknown, "File is outside its library storage root", libraryID, nil
			}
			if !exists {
				for _, old := range previous[id] {
					missingVerified = missingVerified || old == path
				}
				continue
			}
			info, err := os.Stat(resolved)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return AvailabilityUnknown, "File cannot be inspected", libraryID, err
			}
			if !sameStorageDevice(root, resolved, rootInfo, info) {
				return AvailabilityUnknown, "Nested or unverifiable storage volume; configure its own library root", libraryID, nil
			}
			present = present || info.Mode().IsRegular()
			if info.Mode().IsRegular() {
				verified[id] = append(verified[id], path)
			}
		}
		allPresent = allPresent && present
		anyPresent = anyPresent || present
		knownMissing = knownMissing || (missingVerified && !present)
		if missingVerified && !present {
			r.missingFileIDs = append(r.missingFileIDs, id)
		}
	}
	ok, err := s.storageConnected(ctx, libraryID, root, anyPresent, establish)
	if err != nil {
		return AvailabilityUnknown, "Storage identity cannot be verified", libraryID, err
	}
	if !ok {
		return AvailabilityUnknown, "Storage identity unverified or disconnected; no deletion inferred", libraryID, nil
	}
	for id, paths := range previous {
		if _, exists := verified[id]; !exists {
			verified[id] = paths
		}
	}
	payload, err := json.Marshal(verified)
	if err != nil {
		return AvailabilityUnknown, "Cannot record file evidence", libraryID, err
	}
	r.AvailabilityFiles = string(payload)
	if complete && allPresent {
		return AvailabilityPresent, "All currently due requested files verified on connected storage", libraryID, nil
	}
	if knownMissing {
		return AvailabilityMissing, "Connected storage verified; previously imported requested files are missing", libraryID, nil
	}
	return AvailabilityUnknown, "Import records or required metadata are incomplete; deletion is not confirmed", libraryID, nil
}

func pathInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Resolve the closest existing ancestor as well as the missing suffix, so a
// deleted directory and a path through a symlink receive the same root checks.
func resolveEvidencePath(path string) (string, bool, error) {
	candidate := path
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, len(suffix) == 0, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}
		if info, statErr := os.Lstat(candidate); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", false, fmt.Errorf("unresolved storage symlink %s", candidate)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", false, err
		}
		suffix = append(suffix, filepath.Base(candidate))
		candidate = parent
	}
}

func (s *Store) storageConnected(ctx context.Context, libraryID, root string, anyPresent, establish bool) (bool, error) {
	var savedRoot, token string
	err := s.db.QueryRowContext(ctx, `SELECT root_path,token FROM request_storage_anchors WHERE library_id=?`, libraryID).Scan(&savedRoot, &token)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	marker := filepath.Join(root, storageMarker)
	if errors.Is(err, sql.ErrNoRows) {
		if !anyPresent {
			return false, nil
		}
		if !establish {
			return true, nil
		}
		// A marker is established only with a physically verified media file.
		// Never recreate it after loss: an empty unmounted directory must not
		// become evidence that every title was deleted.
		token = uuid.NewString()
		f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- fixed marker in the verified library root.
		if errors.Is(err, os.ErrExist) {
			return false, fmt.Errorf("unregistered storage marker at %s; administrator inspection required", marker)
		}
		if err != nil {
			return false, err
		}
		_, writeErr := f.WriteString(token)
		closeErr := f.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return false, err
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO request_storage_anchors(library_id,root_path,token) VALUES(?,?,?)`, libraryID, root, token)
		return err == nil, err
	}
	if root != savedRoot {
		return false, nil
	}
	actual, err := os.ReadFile(marker) // #nosec G304 -- fixed marker in the verified library root.
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil && string(actual) == token, err
}
