//go:build !windows

package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ebenderooock/loom/internal/updates"
)

// checkedPublisher retains metadata backoff across short-lived timer processes.
// Its directory is the root-owned state directory, never an application setting.
type checkedPublisher struct {
	*updates.Checker
	stateDir string
}

func (p *checkedPublisher) Discover(ctx context.Context, channel string) (updates.Candidate, error) {
	next := map[string]time.Time{}
	err := readJSON(filepath.Join(p.stateDir, "next-check.json"), &next)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return updates.Candidate{}, err
	}
	if time.Now().Before(next[channel]) {
		return updates.Candidate{}, errors.New("native metadata check is throttled until its saved next-check time")
	}
	candidate, err := p.Checker.Discover(ctx, channel)
	next[channel] = p.Checker.Check(ctx, channel).NextCheck
	if writeErr := atomicJSON(p.stateDir, "next-check.json", next); writeErr != nil {
		return updates.Candidate{}, writeErr
	}
	return candidate, err
}
