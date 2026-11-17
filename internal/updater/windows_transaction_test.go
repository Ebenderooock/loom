//go:build windows

package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/ebenderooock/loom/internal/updates"
)

// Filesystem integration only: never enrolls a service, executes an update,
// edits the real Loom layout, or bypasses the production activation barrier.
type winTestControl struct {
	r          Runtime
	stopped    bool
	failHealth bool
}

func (c *winTestControl) validate(context.Context) error           { return nil }
func (c *winTestControl) runtime(context.Context) (Runtime, error) { return c.r, nil }
func (c *winTestControl) captureToken() error                      { return nil }
func (c *winTestControl) releaseToken()                            {}
func (c *winTestControl) stop(context.Context) error               { c.stopped = true; return nil }
func (c *winTestControl) start(context.Context) error              { c.stopped = false; return nil }
func (c *winTestControl) quiescent(context.Context) error {
	if !c.stopped {
		return errors.New("test service still running")
	}
	return nil
}
func (c *winTestControl) snapshotDB(ctx context.Context, dest string) error {
	var data bytes.Buffer
	if err := vacuumToStream(ctx, filepath.ToSlash(DBPath), filepath.Join(DataDir, "vacuum.db"), &data); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(DataDir, "vacuum.db")); err != nil {
		return err
	}
	return os.WriteFile(dest, data.Bytes(), 0600)
}
func (c *winTestControl) healthy(_ context.Context, want Identity) error {
	if c.failHealth && want.Version == "1.1.0" {
		if err := os.WriteFile(filepath.Join(ConfigDir, "migration-added"), []byte("discard"), 0600); err != nil {
			return err
		}
		db, err := sql.Open("sqlite", DBPath)
		if err != nil {
			return err
		}
		_, err = db.Exec("UPDATE sample SET value='migrated'")
		return errors.Join(err, db.Close(), errors.New("simulated candidate health failure"))
	}
	c.r.Identity = want
	return nil
}

type winTestPublisher struct {
	candidate updates.Candidate
	archive   []byte
}

func (p *winTestPublisher) Discover(context.Context, string) (updates.Candidate, error) {
	return p.candidate, nil
}
func (p *winTestPublisher) Ahead(context.Context, string, string) bool { return false }
func (p *winTestPublisher) Download(_ context.Context, _ updates.Candidate, platform string, w io.Writer) error {
	if platform != "windows-"+runtime.GOARCH {
		return errors.New("wrong Windows platform")
	}
	_, err := w.Write(p.archive)
	return err
}

func winTestTransaction(t *testing.T) (*winTransaction, *winTestControl) {
	t.Helper()
	if !winAdmin() {
		t.Skip("native NTFS descriptor/volume-flush tests require elevated Windows")
	}
	if err := winEnablePrivileges(); err != nil {
		t.Fatal(err)
	}
	// Use a system-managed root whose ancestors are not writable by ordinary
	// accounts. ProgramData may grant users create/write rights, which is
	// correctly rejected by winOpen's production ancestor checks.
	parent, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(parent, "LoomUpdaterTest-"+hex.EncodeToString(id))
	if err := winCreateDir(base, winSDDL("FRFX", true), windows.FILE_GENERIC_READ|windows.FILE_GENERIC_EXECUTE, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(base); err != nil {
			t.Error(err)
		}
	})
	oldBin, oldConfig, oldData, oldDB, oldState, oldToken, oldMarker := BinDir, ConfigDir, DataDir, DBPath, StateDir, tokenPath, markerPath
	t.Cleanup(func() {
		BinDir, ConfigDir, DataDir, DBPath, StateDir, tokenPath, markerPath = oldBin, oldConfig, oldData, oldDB, oldState, oldToken, oldMarker
	})
	BinDir, ConfigDir, DataDir, StateDir = filepath.Join(base, "bin"), filepath.Join(base, "config"), filepath.Join(base, "data"), filepath.Join(base, "updater")
	DBPath = filepath.Join(DataDir, "loom.db")
	tokenPath, markerPath = filepath.Join(BinDir, "updater-token"), filepath.Join(BinDir, "updater-managed")
	if err := winCreateDir(BinDir, winSDDL("FRFX", true), windows.FILE_GENERIC_READ|windows.FILE_GENERIC_EXECUTE, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{ConfigDir, DataDir} {
		if err := winCreateDir(path, winSDDL("0x1301bf", true), 0x1301bf, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := winCreateDir(StateDir, winSDDL("", true), 0, true); err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf("config_dir: '%s'\ndata_dir: '%s'\nhttp:\n  addr: '127.0.0.1:1925'\nstorage:\n  engine: sqlite\n  sqlite:\n    path: '%s'\n", ConfigDir, DataDir, DBPath)
	if err := os.WriteFile(filepath.Join(ConfigDir, "loom.yaml"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(BinDir, "loom.exe"), []byte("old executable"), 0600); err != nil {
		t.Fatal(err)
	}
	settings := updates.Settings{Channel: "stable", AutoInstall: true}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", DBPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE TABLE schema_meta (key TEXT PRIMARY KEY,value TEXT);CREATE TABLE sample(value TEXT);INSERT INTO sample VALUES('before')")
	if err == nil {
		_, err = db.Exec("INSERT INTO schema_meta VALUES('updates.settings',?)", string(encoded))
	}
	if err = errors.Join(err, db.Close()); err != nil {
		t.Fatal(err)
	}
	before := Identity{"1.0.0", strings.Repeat("a", 40), "refs/tags/v1.0.0", "stable"}
	if err := winAtomicJSON(StateDir, "state.json", state{Installed: before}); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	member, err := z.Create("loom.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.Write([]byte("new executable")); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	c := &winTestControl{r: Runtime{Identity: before, Native: true, Ready: true, Engine: "sqlite", Settings: settings}}
	p := &winTestPublisher{candidate: updates.Candidate{Manifest: updates.Manifest{SchemaVersion: 1, Version: "1.1.0", Channel: "stable", SourceRef: "refs/tags/v1.1.0", SourceSHA: strings.Repeat("b", 40)}}, archive: archive.Bytes()}
	return &winTransaction{service: c, publisher: p}, c
}

func TestWindowsNTFSTransactionAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint("rollback=", fail), func(t *testing.T) {
			tx, c := winTestTransaction(t)
			c.failHealth = fail
			before, err := os.ReadFile(filepath.Join(ConfigDir, "loom.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			err = tx.run(t.Context())
			if (err != nil) != fail {
				t.Fatalf("transaction=%v want rollback=%v", err, fail)
			}
			want := "new executable"
			if fail {
				want = "old executable"
			}
			binary, err := os.ReadFile(filepath.Join(BinDir, "loom.exe"))
			if err != nil || string(binary) != want {
				t.Fatalf("binary=%q %v", binary, err)
			}
			if err := winTrusted(filepath.Join(BinDir, "loom.exe"), windows.FILE_GENERIC_READ|windows.FILE_GENERIC_EXECUTE, false); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(filepath.Join(ConfigDir, "loom.yaml"))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("configuration not preserved")
			}
			if _, err := os.Stat(filepath.Join(ConfigDir, "migration-added")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("migration-added file survived rollback")
			}
			db, err := sql.Open("sqlite", DBPath)
			if err != nil {
				t.Fatal(err)
			}
			var value string
			err = errors.Join(db.QueryRow("SELECT value FROM sample").Scan(&value), db.Close())
			if err != nil || value != "before" {
				t.Fatalf("restored SQLite=%q %v", value, err)
			}
			if _, err := os.Stat(filepath.Join(StateDir, "journal.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("settled journal retained")
			}
			if c.stopped {
				t.Fatal("old/healthy service not restarted")
			}
		})
	}
}

func TestWindowsNTFSInterruptedRecovery(t *testing.T) {
	tx, c := winTestTransaction(t)
	work := filepath.Join(StateDir, "transaction")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(work, "new-binary"), []byte("new executable"), 0600); err != nil {
		t.Fatal(err)
	}
	c.stopped = true
	j, err := tx.snapshot(t.Context(), state{Installed: c.r.Identity}, Identity{"1.1.0", strings.Repeat("b", 40), "refs/tags/v1.1.0", "stable"}, work)
	if err != nil {
		t.Fatal(err)
	}
	if err := winAtomicJSON(StateDir, "journal.json", j); err != nil {
		t.Fatal(err)
	}
	if err := winReplace(filepath.Join(BinDir, "loom.exe"), filepath.Join(work, "new-binary"), j.Binary); err != nil {
		t.Fatal(err)
	}
	// Simulated process/power loss after replacement, before start/commit.
	if err := tx.recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(BinDir, "loom.exe"))
	if err != nil || string(got) != "old executable" {
		t.Fatal("old image not recovered")
	}
	var saved state
	if err := readJSON(filepath.Join(StateDir, "state.json"), &saved); err != nil || saved.FailedSHA != j.Target.SHA {
		t.Fatal("failed build not blocked")
	}
}

func TestWindowsSnapshotIncludesCommittedWAL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "with spaces")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.db")
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL;PRAGMA wal_autocheckpoint=0;CREATE TABLE payload(value TEXT);INSERT INTO payload VALUES('committed-wal')"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(source + "-wal"); err != nil || info.Size() == 0 {
		t.Fatal("fixture requires committed WAL")
	}
	var out bytes.Buffer
	if err := vacuumToStream(t.Context(), filepath.ToSlash(source), filepath.Join(dir, "snapshot 'quoted'.db"), &out); err != nil {
		t.Fatal(err)
	}
	restore := filepath.Join(dir, "restored.db")
	if err := os.WriteFile(restore, out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	check, err := sql.Open("sqlite", restore)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var got string
	if err := check.QueryRow("SELECT value FROM payload").Scan(&got); err != nil || got != "committed-wal" {
		t.Fatalf("inconsistent Windows WAL snapshot %q %v", got, err)
	}
}

func TestWindowsNTFSRejectsHardlinksAndStreams(t *testing.T) {
	_, _ = winTestTransaction(t)
	exe := filepath.Join(BinDir, "loom.exe")
	link := filepath.Join(BinDir, "hardlink.exe")
	if err := os.Link(exe, link); err != nil {
		t.Fatal(err)
	}
	if f, err := winOpen(exe, 0); err == nil {
		f.Close()
		t.Fatal("hard-linked executable accepted")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(ConfigDir, "with-stream")
	if err := os.WriteFile(file, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file+":private", []byte("stream"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := winCopySnapshot(file, filepath.Join(StateDir, "invalid-copy"), "with-stream", 1024); err == nil {
		t.Fatal("alternate stream accepted")
	}
}
