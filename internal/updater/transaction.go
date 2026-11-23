//go:build !windows

package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ebenderooock/loom/internal/updates"
)

type control interface {
	validate(context.Context) error
	runtime(context.Context) (Runtime, error)
	stop(context.Context) error
	quiescent(context.Context) error
	start(context.Context) error
	healthy(context.Context, Identity) error
	snapshotDB(context.Context, string) error
}

type layout struct{ bin, config, data, state string }
type engine struct {
	paths     layout
	platform  string
	service   control
	publisher publisher
}

type fileMeta struct {
	Path string `json:"path"`
	Mode uint32 `json:"mode"`
	UID  int    `json:"uid"`
	GID  int    `json:"gid"`
	Dir  bool   `json:"dir"`
}
type journal struct {
	Before state      `json:"before"`
	Target Identity   `json:"target"`
	Config []fileMeta `json:"config"`
	DB     fileMeta   `json:"db"`
	Binary fileMeta   `json:"binary"`
}

func (e *engine) run(ctx context.Context) error {
	if err := e.service.validate(ctx); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(e.paths.state, "journal.json")); err == nil {
		return e.recover(ctx, true)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var before state
	if err := readJSON(filepath.Join(e.paths.state, "state.json"), &before); err != nil {
		return err
	}
	r, err := e.service.runtime(ctx)
	if err != nil {
		return err
	}
	if !r.Native || !r.Ready || r.Engine != "sqlite" {
		return errors.New("native SQLite service is not ready")
	}
	if !r.Settings.AutoInstall {
		return nil
	}
	if !validConsent(r.Settings) {
		return errors.New("invalid update channel consent")
	}
	if !sameIdentity(r.Identity, before.Installed) {
		return errors.New("installed version changed; root re-enrollment required")
	}
	candidate, err := e.publisher.Discover(ctx, r.Settings.Channel)
	if err != nil {
		return err
	}
	m := candidate.Manifest
	target := Identity{Version: m.Version, SHA: m.SourceSHA, Ref: m.SourceRef, Channel: m.Channel}
	if target.SHA == before.FailedSHA {
		return errors.New("this build previously failed; root re-enrollment required to retry")
	}
	if !e.newer(ctx, before.Installed, target) {
		return nil
	}
	work := filepath.Join(e.paths.state, "transaction")
	if err := os.RemoveAll(work); err != nil {
		return err
	} // root-owned, never service-writable
	if err := os.Mkdir(work, 0700); err != nil {
		return err
	}
	archive, err := os.OpenFile(filepath.Join(work, "archive"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) // #nosec G304 -- fixed root-private transaction directory, no caller paths
	if err != nil {
		return err
	}
	err = e.publisher.Download(ctx, candidate, e.platform, archive)
	if err == nil {
		err = archive.Sync()
	}
	closeErr := archive.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := extractBinary(filepath.Join(work, "archive"), filepath.Join(work, "new-binary")); err != nil {
		return err
	}
	if err := e.service.validate(ctx); err != nil {
		return err
	}
	if err := e.service.stop(ctx); err != nil {
		return err
	}
	// Any pre-journal failure has not changed the installation. Restart the old
	// service, even when the caller's context was cancelled by service shutdown.
	j, err := e.snapshot(ctx, before, target, work)
	if err == nil {
		err = atomicJSON(e.paths.state, "journal.json", j)
	}
	if err != nil {
		restartCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		// Rename may publish the complete journal before directory fsync fails.
		// Never resume service with a pending journal: recovery must settle it
		// first, or later recovery could discard intervening application writes.
		if _, journalErr := os.Stat(filepath.Join(e.paths.state, "journal.json")); journalErr == nil {
			return errors.Join(err, e.recover(restartCtx, true))
		} else if !errors.Is(journalErr, os.ErrNotExist) {
			return errors.Join(err, journalErr)
		}
		return errors.Join(err, e.service.start(restartCtx))
	}
	if err = replaceFile(e.paths.bin, "loom", filepath.Join(work, "new-binary"), j.Binary); err == nil {
		err = e.service.start(ctx)
	}
	if err == nil {
		err = e.service.healthy(ctx, target)
	}
	if err == nil {
		err = atomicJSON(e.paths.state, "state.json", state{Installed: target})
	}
	if err == nil {
		err = removeDurable(e.paths.state, "journal.json")
	}
	if err == nil {
		return nil
	}
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
	defer cancel()
	return errors.Join(err, e.recover(recoveryCtx, true))
}

func (e *engine) newer(ctx context.Context, old, next Identity) bool {
	return candidateNewer(ctx, e.publisher, old, next)
}

func (e *engine) snapshot(ctx context.Context, before state, target Identity, work string) (journal, error) {
	j := journal{Before: before, Target: target}
	var err error
	j.Config, err = snapshotTree(e.paths.config, filepath.Join(work, "config"))
	if err != nil {
		return j, err
	}
	if err := validateConfig(filepath.Join(work, "config", "loom.yaml")); err != nil {
		return j, err
	}
	data, err := os.OpenRoot(e.paths.data)
	if err != nil {
		return j, err
	}
	defer func() { _ = data.Close() }()
	info, err := data.Lstat("loom.db")
	if err != nil {
		return j, err
	}
	var links uint64
	j.DB, links, err = metadata("loom.db", info)
	if err != nil || links != 1 || !info.Mode().IsRegular() || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return j, errors.New("invalid database file")
	}
	dbFile, err := data.Open("loom.db")
	if err != nil {
		return j, err
	}
	err = noACL(dbFile)
	_ = dbFile.Close()
	if err != nil {
		return j, err
	}
	// The platform invokes the fixed helper as the unprivileged loom account.
	// SQLite VACUUM INTO takes a real read transaction, includes committed WAL
	// data, and emits a standalone DB into this root-private backup. Root never
	// opens the service-writable SQLite/WAL/SHM through the SQLite engine.
	if err := e.service.snapshotDB(ctx, filepath.Join(work, "loom.db")); err != nil {
		return j, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(work, "loom.db")+"?mode=rw")
	if err != nil {
		return j, err
	}
	defer func() { _ = db.Close() }()
	var check string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		return j, errors.New("SQLite snapshot integrity check failed")
	}
	var raw string
	if err := db.QueryRow(`SELECT value FROM schema_meta WHERE key = 'updates.settings'`).Scan(&raw); err != nil {
		return j, err
	}
	var settings updates.Settings
	if json.Unmarshal([]byte(raw), &settings) != nil || !settings.AutoInstall ||
		!validConsent(settings) || settings.Channel != target.Channel {
		return j, errors.New("update consent changed before snapshot")
	}
	if err := db.Close(); err != nil {
		return j, err
	}
	// Backup the old executable as data; never execute downloaded code as root.
	bin, err := os.OpenRoot(e.paths.bin)
	if err != nil {
		return j, err
	}
	defer func() { _ = bin.Close() }()
	j.Binary, err = snapshotFile(bin, "loom", filepath.Join(work, "old-binary"), 512<<20)
	if err != nil {
		return j, err
	}
	for _, name := range []string{"loom.db", "old-binary", "new-binary"} {
		if err := syncFile(filepath.Join(work, name)); err != nil {
			return j, err
		}
	}
	return j, syncDir(work)
}

// recover is idempotent. journal.json remains durable until all restoration
// succeeds; the boot dependency prevents Loom starting against partial state.
func (e *engine) recover(ctx context.Context, start bool) error {
	var j journal
	if err := readJSON(filepath.Join(e.paths.state, "journal.json"), &j); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := e.service.validate(ctx); err != nil {
		return err
	}
	if start {
		if err := e.service.stop(ctx); err != nil {
			return err
		}
	} else if err := e.service.quiescent(ctx); err != nil {
		return err
	}
	work := filepath.Join(e.paths.state, "transaction")
	if err := restoreTree(e.paths.config, filepath.Join(work, "config"), j.Config); err != nil {
		return err
	}
	if err := replaceFile(e.paths.data, "loom.db", filepath.Join(work, "loom.db"), j.DB); err != nil {
		return err
	}
	root, err := os.OpenRoot(e.paths.data)
	if err != nil {
		return err
	}
	for _, name := range []string{"loom.db-wal", "loom.db-shm"} {
		if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = root.Close()
			return err
		}
	}
	_ = root.Close()
	if err := syncDir(e.paths.data); err != nil {
		return err
	}
	if err := replaceFile(e.paths.bin, "loom", filepath.Join(work, "old-binary"), j.Binary); err != nil {
		return err
	}
	j.Before.FailedSHA = j.Target.SHA
	if err := atomicJSON(e.paths.state, "state.json", j.Before); err != nil {
		return err
	}
	// Boot recovery must not start a unit which is waiting for this oneshot.
	if !start {
		return removeDurable(e.paths.state, "journal.json")
	}
	if err := e.service.start(ctx); err != nil {
		return err
	}
	if err := e.service.healthy(ctx, j.Before.Installed); err != nil {
		return err
	}
	return removeDurable(e.paths.state, "journal.json")
}

func extractBinary(archivePath, dest string) error {
	f, err := os.Open(archivePath) // #nosec G304 -- fixed root-private downloaded archive, never a request path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(io.LimitReader(gz, 1<<30))
	found := false
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		// Nothing other than the one binary is ever extracted. Do not honor
		// archive ownership, links, directories, or executable mode bits.
		if h.Name != "loom" && h.Name != "./loom" {
			continue
		}
		if found || h.Typeflag != tar.TypeReg || h.Size <= 0 || h.Size > 512<<20 {
			return errors.New("invalid or duplicate Loom binary in archive")
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) // #nosec G304 -- fixed root-private binary staging destination
		if err != nil {
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(tr, (512<<20)+1))
		syncErr := out.Sync()
		closeErr := out.Close()
		if n != h.Size {
			return errors.New("truncated binary")
		}
		if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
			return err
		}
		found = true
	}
	if !found {
		return errors.New("release archive has no regular Loom binary")
	}
	return nil
}
