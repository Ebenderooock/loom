//go:build darwin

package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/sys/unix"

	"github.com/ebenderooock/loom/internal/updates"
)

// Main accepts only installer/launchd operations, never commands, paths or
// candidate URLs. The snapshot operation is exclusively unprivileged.
func Main(ctx context.Context, args []string) error {
	if len(args) == 1 && args[0] == "snapshot" {
		return macSnapshotChild(ctx)
	}
	if os.Geteuid() != 0 {
		return errors.New("native updater requires root under its installed launch daemon")
	}
	if len(args) > 1 {
		return errors.New("only fixed enroll/recover/preflight/daemon operations are supported")
	}
	mode := ""
	if len(args) == 1 {
		mode = args[0]
	}
	switch mode {
	case "", "enroll", "recover", "preflight", "daemon":
	default:
		return errors.New("unknown fixed native updater operation")
	}
	if mode == "preflight" {
		return macPreflight()
	}
	if mode == "enroll" {
		if _, err := os.Stat(StateDir + "/journal.json"); err == nil {
			return errors.New("recover the pending transaction before enrollment")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := macPreflight(); err != nil {
			return err
		}
		// Establish only the private directory before taking the daemon lock.
		// Do not rewrite enrollment/token files while another helper is active.
		if err := os.Mkdir(StateDir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	if err := trusted(StateDir, 0700, true); err != nil {
		return err
	}
	lock, err := os.OpenFile(StateDir+"/lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("another native helper owns recovery")
	}
	if mode == "enroll" {
		if _, err := os.Stat(StateDir + "/journal.json"); !errors.Is(err, os.ErrNotExist) {
			return errors.New("recovery must settle before enrollment")
		}
		if err := macPrepare(); err != nil {
			return err
		}
	}
	token, err := readToken()
	if err != nil {
		return err
	}
	gate := &startGate{}
	s := &launchd{token: token, gate: gate, enrolling: mode == "enroll"}
	if err := s.validate(ctx); err != nil {
		return err
	}
	uid, gid, err := macAccount()
	if err != nil {
		return err
	}
	listener, err := gate.listen(uid, gid)
	if err != nil {
		return err
	}
	defer func() { gate.allow.Store(false); _ = listener.Close() }()
	e := engine{paths: layout{BinDir, ConfigDir, DataDir, StateDir}, platform: "darwin-" + runtime.GOARCH, service: s,
		publisher: &checkedPublisher{Checker: updates.NewChecker("dev"), stateDir: StateDir}}
	if mode == "enroll" {
		if err := s.start(ctx); err != nil {
			return err
		}
		deadline, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		for {
			r, err := s.runtime(deadline)
			if err == nil && r.Native && r.Ready && r.Engine == "sqlite" && updates.ValidSource(r.Version, r.Ref, r.SHA, r.Channel) && s.processMatches(deadline) {
				if err := atomicJSON(StateDir, "state.json", state{Installed: r.Identity}); err != nil {
					return err
				}
				return writeMacProof(BinDir+"/updater-enrolled", updates.TrustFingerprint())
			}
			select {
			case <-deadline.Done():
				return errors.New("enrollment requires the ready native SQLite release with valid source metadata")
			case <-time.After(time.Second):
			}
		}
	}
	if mode == "recover" {
		return e.recover(ctx, true)
	}
	for {
		cycle, cancel := context.WithTimeout(ctx, 15*time.Minute)
		// Startup gate remains closed until recovery has settled any journal.
		if _, err := os.Stat(filepath.Join(StateDir, "journal.json")); err == nil {
			err = e.recover(cycle, true)
			if err != nil {
				cancel()
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			cancel()
			return err
		} else if err := s.start(cycle); err != nil {
			cancel()
			return err
		}
		err := e.run(cycle)
		cancel()
		if mode != "daemon" {
			return err
		}
		if err != nil {
			// A pending journal must be retried promptly by launchd, not after
			// the next hourly release check. Do not log service-controlled data.
			if _, pending := os.Stat(StateDir + "/journal.json"); !errors.Is(pending, os.ErrNotExist) {
				return errors.New("native update requires recovery")
			}
			fmt.Fprintln(os.Stderr, "Native update check deferred; installation was not advanced.")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Hour):
		}
	}
}
