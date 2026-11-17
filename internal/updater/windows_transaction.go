//go:build windows

package updater

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ebenderooock/loom/internal/updates"
)

// Separate filesystem implementation: never route Windows through Unix Chown,
// os.Rename replacement, tar extraction or directory-fsync assumptions.
type winTransaction struct {
	service   winTransactionControl
	publisher publisher
}
type winTransactionControl interface {
	validate(context.Context) error
	runtime(context.Context) (Runtime, error)
	stop(context.Context) error
	quiescent(context.Context) error
	start(context.Context) error
	healthy(context.Context, Identity) error
	snapshotDB(context.Context, string) error
	captureToken() error
	releaseToken()
}
type winJournal struct {
	Before state     `json:"before"`
	Target Identity  `json:"target"`
	Config []winMeta `json:"config"`
	DB     winMeta   `json:"db"`
	Binary winMeta   `json:"binary"`
}

func (t *winTransaction) run(ctx context.Context) error {
	if err := t.service.validate(ctx); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(StateDir, "journal.json")); err == nil {
		return t.recover(ctx)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var before state
	if err := readJSON(filepath.Join(StateDir, "state.json"), &before); err != nil {
		return err
	}
	r, err := t.service.runtime(ctx)
	if err != nil {
		return err
	}
	if !r.Native || !r.Ready || r.Engine != "sqlite" {
		return errors.New("native SQLite service not ready")
	}
	if !r.Settings.AutoInstall {
		return nil
	}
	if !validConsent(r.Settings) || !sameIdentity(r.Identity, before.Installed) {
		return errors.New("consent or installed identity differs from enrollment")
	}
	candidate, err := t.publisher.Discover(ctx, r.Settings.Channel)
	if err != nil {
		return err
	}
	m := candidate.Manifest
	target := Identity{m.Version, m.SourceSHA, m.SourceRef, m.Channel}
	if target.SHA == before.FailedSHA {
		return errors.New("failed Windows build is blocked until administrator re-enrollment")
	}
	if !candidateNewer(ctx, t.publisher, before.Installed, target) {
		return nil
	}
	work := filepath.Join(StateDir, "transaction")
	if err := os.RemoveAll(work); err != nil {
		return err
	}
	if err := os.Mkdir(work, 0700); err != nil {
		return err
	}
	archive, err := os.OpenFile(filepath.Join(work, "archive.zip"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) // #nosec G304 -- fixed root-private transaction staging path
	if err != nil {
		return err
	}
	err = t.publisher.Download(ctx, candidate, "windows-"+runtime.GOARCH, archive)
	err = errors.Join(err, archive.Sync(), archive.Close())
	if err != nil {
		return err
	}
	if err := winExtractZIP(filepath.Join(work, "archive.zip"), filepath.Join(work, "new-binary")); err != nil {
		return err
	}
	if err := t.service.validate(ctx); err != nil {
		return err
	}
	if err := t.service.captureToken(); err != nil {
		return err
	}
	defer t.service.releaseToken()
	if err := t.service.stop(ctx); err != nil {
		return err
	}
	j, err := t.snapshot(ctx, before, target, work)
	if err == nil {
		err = winAtomicJSON(StateDir, "journal.json", j)
	}
	if err != nil {
		resume, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		if _, pending := os.Stat(filepath.Join(StateDir, "journal.json")); pending == nil {
			return errors.Join(err, t.recover(resume))
		} else if !errors.Is(pending, os.ErrNotExist) {
			return errors.Join(err, pending)
		}
		return errors.Join(err, t.service.start(resume))
	}
	if err = winReplace(filepath.Join(BinDir, "loom.exe"), filepath.Join(work, "new-binary"), j.Binary); err == nil {
		err = t.service.start(ctx)
	}
	if err == nil {
		err = t.service.healthy(ctx, target)
	}
	if err == nil {
		err = winAtomicJSON(StateDir, "state.json", state{Installed: target})
	}
	if err == nil {
		err = winRemoveJournal(StateDir)
	}
	if err == nil {
		return nil
	}
	recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	return errors.Join(err, t.recover(recovery))
}
func (t *winTransaction) snapshot(ctx context.Context, before state, target Identity, work string) (winJournal, error) {
	j := winJournal{Before: before, Target: target}
	if err := t.service.quiescent(ctx); err != nil {
		return j, err
	}
	var err error
	j.Config, err = winSnapshotTree(ConfigDir, filepath.Join(work, "config"))
	if err != nil {
		return j, err
	}
	if err := validateConfig(filepath.Join(work, "config", "loom.yaml")); err != nil {
		return j, err
	}
	f, err := winOpen(DBPath, 0x01000000)
	if err != nil {
		return j, err
	} // ACCESS_SYSTEM_SECURITY
	j.DB, err = winMetadata(f, "loom.db")
	_ = f.Close()
	if err != nil {
		return j, err
	}
	if err := t.service.snapshotDB(ctx, filepath.Join(work, "loom.db")); err != nil {
		return j, err
	}
	if err := winCheckSnapshot(filepath.Join(work, "loom.db"), target.Channel); err != nil {
		return j, err
	}
	if err := t.service.quiescent(ctx); err != nil {
		return j, err
	}
	j.Binary, err = winCopySnapshot(filepath.Join(BinDir, "loom.exe"), filepath.Join(work, "old-binary"), "loom.exe", 512<<20)
	if err != nil {
		return j, err
	}
	return j, winFlushVolume(work)
}
func winCheckSnapshot(path, channel string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	var check string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		return errors.New("inconsistent Windows SQLite snapshot")
	}
	var raw string
	if err := db.QueryRow("SELECT value FROM schema_meta WHERE key = 'updates.settings'").Scan(&raw); err != nil {
		return err
	}
	var consent updates.Settings
	if json.Unmarshal([]byte(raw), &consent) != nil || !consent.AutoInstall || !validConsent(consent) || consent.Channel != channel {
		return errors.New("update consent changed before stopped snapshot")
	}
	return nil
}
func (t *winTransaction) recover(ctx context.Context) error {
	var j winJournal
	if err := readJSON(filepath.Join(StateDir, "journal.json"), &j); err != nil {
		return err
	}
	if err := t.service.validate(ctx); err != nil {
		return err
	}
	if err := t.service.stop(ctx); err != nil {
		return err
	}
	work := filepath.Join(StateDir, "transaction")
	if err := winRestoreTree(ConfigDir, filepath.Join(work, "config"), j.Config); err != nil {
		return err
	}
	if err := winReplace(DBPath, filepath.Join(work, "loom.db"), j.DB); err != nil {
		return err
	}
	for _, name := range []string{"loom.db-wal", "loom.db-shm"} {
		path := filepath.Join(DataDir, name)
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		f, err := winOpen(path, 0)
		if err != nil {
			return err
		}
		_ = f.Close()
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	if err := winFlushVolume(DataDir); err != nil {
		return err
	}
	if err := winReplace(filepath.Join(BinDir, "loom.exe"), filepath.Join(work, "old-binary"), j.Binary); err != nil {
		return err
	}
	j.Before.FailedSHA = j.Target.SHA
	if err := winAtomicJSON(StateDir, "state.json", j.Before); err != nil {
		return err
	}
	// start announces SCM Running only after restoration. Before this point the
	// helper dependency plus closed live gate prevent any database migrations.
	if err := t.service.start(ctx); err != nil {
		return err
	}
	if err := t.service.healthy(ctx, j.Before.Installed); err != nil {
		return err
	}
	return winRemoveJournal(StateDir)
}
