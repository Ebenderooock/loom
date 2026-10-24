package downloads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ebenderooock/loom/internal/indexers"
)

// SearchAcquisition extends existing provider search diagnostics with durable
// per-query failure budgets. Service.Search still owns quota admission,
// capability checks, circuit breakers and concurrent provider fanout.
func (r *Registry) SearchAcquisition(ctx context.Context, svc *indexers.Service, q indexers.Query, media AddRequest) (indexers.AggregatedResults, error) {
	r.mu.RLock()
	history := r.history
	r.mu.RUnlock()
	if history == nil {
		return svc.Search(ctx, q, nil, 0), nil
	}
	encoded, err := json.Marshal(q)
	if err != nil {
		return indexers.AggregatedResults{}, err
	}
	attempts := make(map[string]string)
	ids := make([]string, 0)
	deferDiagnostics := make([]indexers.IndexerDiagnostic, 0)
	for _, provider := range svc.Registry().List() {
		req := media
		req.acquisitionScope = "search"
		req.IndexerID = provider.ID()
		sum := sha256.Sum256([]byte(req.MediaType + "\x00" + req.MediaID + "\x00" + provider.ID() + "\x00" + string(encoded)))
		req.Infohash = hex.EncodeToString(sum[:])
		id, beginErr := history.beginAcquisition(ctx, req, "", time.Now())
		if errors.Is(beginErr, ErrAcquisitionDeferred) {
			deferDiagnostics = append(deferDiagnostics, indexers.IndexerDiagnostic{
				ID: provider.ID(), Name: provider.Name(), Status: "skipped", ErrorMessage: "acquisition retry deferred; see download history",
			})
			continue
		}
		if beginErr != nil {
			return indexers.AggregatedResults{}, beginErr
		}
		attempts[provider.ID()] = id
		ids = append(ids, provider.ID())
	}
	// An empty ID list means "all providers" to Service.Search. Never expand
	// a fully deferred set into an unrestricted search.
	if len(ids) == 0 {
		return indexers.AggregatedResults{
			Errors:      map[string]string{},
			Diagnostics: &indexers.SearchDiagnostics{Indexers: deferDiagnostics},
		}, nil
	}
	agg := svc.Search(ctx, q, ids, 0)
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	for providerID, id := range attempts {
		if _, failed := agg.Errors[providerID]; failed {
			if err := history.finishAcquisition(saveCtx, id, "provider_search", AddResult{}, true, false, time.Now()); err != nil {
				return agg, fmt.Errorf("persist provider search failure: %w", err)
			}
			continue
		}
		skipped := false
		if agg.Diagnostics != nil {
			for _, diagnostic := range agg.Diagnostics.Indexers {
				if diagnostic.ID == providerID && diagnostic.Status == "skipped" {
					skipped = true
					break
				}
			}
		}
		if skipped {
			// Preserve the previous failure budget when capability/circuit/quota
			// filtering prevented an actual query.
			if _, err := history.db.ExecContext(saveCtx, `DELETE FROM download_history WHERE id = ?`, id); err != nil {
				return agg, fmt.Errorf("release skipped provider admission: %w", err)
			}
			continue
		}
		// Successful queries do not consume a failure budget.
		// Keep the row so earlier failures cannot become the latest admission.
		if _, err := history.db.ExecContext(saveCtx, `UPDATE download_history SET status = 'search_completed',
			stage = 'provider', reason = '', retry_count = 0, completed_at = ? WHERE id = ?`,
			time.Now().UTC().Format(time.RFC3339), id); err != nil {
			return agg, fmt.Errorf("persist provider search outcome: %w", err)
		}
	}
	if agg.Diagnostics == nil {
		agg.Diagnostics = &indexers.SearchDiagnostics{}
	}
	agg.Diagnostics.Indexers = append(agg.Diagnostics.Indexers, deferDiagnostics...)
	return agg, nil
}
