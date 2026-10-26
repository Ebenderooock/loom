package downloads

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/workflows"
)

// ErrAcquisitionDeferred means the release is already admitted, blocked, or
// awaiting a retry window or an administrator's explicit recovery decision.
var ErrAcquisitionDeferred = errors.New("acquisition deferred; see download history")

const maxAcquisitionAttempts = workflows.MaxRetries

func acquisitionIdentity(req AddRequest) (key, releaseHash string) {
	releaseHash = strings.ToLower(req.Infohash)
	if releaseHash == "" {
		magnet := req.Magnet
		if strings.HasPrefix(req.TorrentURL, "magnet:") {
			magnet = req.TorrentURL
		}
		if parsed, err := url.Parse(magnet); err == nil {
			for _, xt := range parsed.Query()["xt"] {
				if strings.HasPrefix(strings.ToLower(xt), "urn:btih:") {
					releaseHash = strings.ToLower(xt[len("urn:btih:"):])
					break
				}
			}
		}
	}
	if len(releaseHash) == 32 {
		if decoded, err := base32.StdEncoding.DecodeString(strings.ToUpper(releaseHash)); err == nil {
			releaseHash = hex.EncodeToString(decoded)
		}
	}
	if releaseHash == "" {
		// Titles, unlike signed download URLs and provider GUIDs, remain stable
		// when an indexer rotates credentials or republishes the same release.
		identity := strings.ToLower(strings.TrimSpace(req.Title))
		if identity == "" {
			identity = req.TorrentURL + "\x00" + req.NZBURL + "\x00" + req.Magnet
			if len(req.RawBytes) > 0 {
				payload := sha256.Sum256(req.RawBytes)
				identity = hex.EncodeToString(payload[:])
			}
		}
		sum := sha256.Sum256([]byte(req.IndexerID + "\x00" + identity))
		releaseHash = hex.EncodeToString(sum[:])
	}
	prefix := "acquisition:"
	if req.acquisitionScope != "" {
		prefix = req.acquisitionScope + ":"
	}
	return prefix + releaseHash, releaseHash
}

func (h *HistoryStore) beginAcquisition(ctx context.Context, req AddRequest, clientID string, now time.Time) (string, error) {
	key, hash := acquisitionIdentity(req)
	id := uuid.NewString()
	mediaID := req.MediaID
	if mediaID == "" {
		mediaID = req.MovieID
	}
	if mediaID == "" {
		mediaID = strings.Join(req.EpisodeIDs, ",")
	}
	at := now.UTC().Format(time.RFC3339)
	result, err := h.db.ExecContext(ctx, `
		INSERT INTO download_history
		(id, download_id, client_id, title, category, status, completed_at,
		 acquisition_key, media_type, media_id, indexer_id, release_hash, stage,
		 reason, attempt, retry_count)
		SELECT ?, '', ?, ?, ?, 'acquiring', ?, ?, ?, ?, ?, ?, 'admission',
		 'Acquisition in progress; interrupted attempts require explicit retry.',
		 COALESCE((SELECT MAX(attempt) FROM download_history WHERE acquisition_key = ?), 0) + 1,
		 COALESCE((SELECT retry_count FROM download_history WHERE acquisition_key = ? ORDER BY attempt DESC LIMIT 1), 0) + 1
		WHERE NOT EXISTS (
		 SELECT 1 FROM download_history WHERE acquisition_key = ?
		 AND attempt = (SELECT MAX(attempt) FROM download_history WHERE acquisition_key = ?)
		 AND status <> 'search_completed'
		 AND (status <> 'acquisition_failed' OR retry_eligible = 0 OR next_retry_at > ?)
		) AND NOT EXISTS (SELECT 1 FROM blocklist WHERE release_hash = ?)`,
		id, clientID, req.Title, req.Category, at, key, req.MediaType, mediaID, req.IndexerID, hash,
		key, key, key, key, at, hash)
	if err != nil {
		return "", fmt.Errorf("persist acquisition admission: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", ErrAcquisitionDeferred
	}
	return id, nil
}

func (h *HistoryStore) finishAcquisition(ctx context.Context, id, stage string, res AddResult, failed, permanent bool, now time.Time) error {
	status, reason, next := "acquired", "", ""
	eligible := false
	if failed {
		status = "acquisition_failed"
		reason = "Download client unavailable or rejected the request."
		if stage == "provider" {
			reason = "Indexer download fetch failed."
		}
		if stage == "provider_search" {
			reason = "Indexer search failed."
		}
		if stage == "validation" {
			reason = "Release has no downloadable payload."
		}
		var count int
		if err := h.db.QueryRowContext(ctx, `SELECT retry_count FROM download_history WHERE id = ?`, id).Scan(&count); err != nil {
			return err
		}
		eligible = !permanent && count < maxAcquisitionAttempts
		if eligible {
			next = now.Add(workflows.RetryBackoff(count)).UTC().Format(time.RFC3339)
		}
	}
	_, err := h.db.ExecContext(ctx, `UPDATE download_history SET status = ?, stage = ?,
		reason = ?, retry_eligible = ?, next_retry_at = ?, completed_at = ?, download_id = ?
		WHERE id = ?`, status, stage, reason, eligible, next, now.UTC().Format(time.RFC3339), res.ItemID, id)
	return err
}

// Acquire persists admission before provider/client side effects. A nil history
// store preserves the lightweight registry used by embedding callers and tests.
// prepare fetches authenticated provider payloads within the same retry budget.
func (r *Registry) Acquire(ctx context.Context, clientID string, req AddRequest, prepare func(*AddRequest) error) (AddResult, error) {
	r.mu.RLock()
	history := r.history
	r.mu.RUnlock()
	var id string
	var err error
	if history != nil {
		id, err = history.beginAcquisition(ctx, req, clientID, time.Now())
		if err != nil {
			return AddResult{}, err
		}
	}
	stage, permanent := "client", false
	var res AddResult
	req.Normalize()
	if req.Magnet == "" && req.TorrentURL == "" && req.NZBURL == "" && len(req.RawBytes) == 0 {
		stage, permanent = "validation", true
		err = errors.New("release has no downloadable payload")
	} else if prepare != nil {
		err = prepare(&req)
		if err != nil {
			stage = "provider"
		}
	}
	if err == nil {
		if client, ok := r.Get(clientID); ok {
			res, err = client.Add(ctx, req)
		} else {
			err = errors.New("download client unavailable")
		}
	}
	if history != nil {
		// A canceled request must not lose the failure record. If persistence
		// fails, leave the admission pending (fail closed, never repeat Add).
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		// A canceled client call may have reached the remote client. Keep its
		// admission pending rather than scheduling an ambiguous duplicate.
		if stage == "client" && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			return AddResult{}, errors.New("acquisition outcome unknown; check client queue and download history")
		}
		if saveErr := history.finishAcquisition(saveCtx, id, stage, res, err != nil, permanent, time.Now()); saveErr != nil {
			return AddResult{}, fmt.Errorf("persist acquisition outcome: %w", saveErr)
		}
		if err != nil {
			// Provider/client error text can contain credentials and response bodies.
			return AddResult{}, fmt.Errorf("acquisition %s failed; see download history", stage)
		}
	}
	return res, err
}

// SetHistoryStore enables durable acquisition admission for every registry caller.
func (r *Registry) SetHistoryStore(h *HistoryStore) {
	r.mu.Lock()
	r.history = h
	r.mu.Unlock()
}
