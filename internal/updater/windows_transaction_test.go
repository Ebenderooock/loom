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
	r                  Runtime
	stopped            bool
	failHealth         bool
	failRollbackHealth bool
	failStop           bool
	starts             int
}

var errWinTestHealth = errors.New("simulated candidate health failure")

func (c *winTestControl) validate(context.Context) error           { return nil }
func (c *winTestControl) runtime(context.Context) (Runtime, error) { return c.r, nil }
func (c *winTestControl) captureToken() error                      { return nil }
func (c *winTestControl) releaseToken()                            {}
func (c *winTestControl) stop(context.Context) error {
	c.stopped = true
	if c.failStop {
		return errors.New("simulated stop failure")
	}
	return nil
}
func (c *winTestControl) start(context.Context) error {
	c.starts++
	c.stopped = false
	return nil
}
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
	if c.failRollbackHealth && want.Version == "1.0.0" {
		c.failRollbackHealth = false
		return errors.New("simulated rollback health failure")
	}
	if c.failHealth && want.Version == "1.1.0" {
		if err := os.WriteFile(filepath.Join(ConfigDir, "migration-added"), []byte("discard"), 0600); err != nil {
			return err
		}
		db, err := sql.Open("sqlite", DBPath)
		if err != nil {
			return err
		}
		_, err = db.Exec("UPDATE sample SET value='migrated'")
		return errors.Join(err, db.Close(), errWinTestHealth)
	}
	c.r.Identity = want
	return nil
}

type winTestPublisher struct {
	candidate   updates.Candidate
	archive     []byte
	downloadErr error
	downloads   int
}

func (p *winTestPublisher) Discover(context.Context, string) (updates.Candidate, error) {
	return p.candidate, nil
}
func (p *winTestPublisher) Ahead(context.Context, string, string) bool { return false }
func (p *winTestPublisher) Download(_ context.Context, _ updates.Candidate, platform string, w io.Writer) error {
	p.downloads++
	if platform != "windows-"+runtime.GOARCH {
		return errors.New("wrong Windows platform")
	}
	_, err := w.Write(p.archive)
	return errors.Join(err, p.downloadErr)
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
	if err := winCreateDir(filepath.Join(ConfigDir, "nested"), winSDDL("0x1301bf", true), 0x1301bf, false); err != nil {
		t.Fatal(err)
	}
	nestedConfig := filepath.Join(ConfigDir, "nested", "settings")
	if err := os.WriteFile(nestedConfig, []byte("preserved nested configuration"), 0600); err != nil {
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
	for _, stmt := range []string{
		"CREATE TABLE schema_meta (key TEXT PRIMARY KEY,value TEXT)",
		"CREATE TABLE sample(value TEXT)",
		"INSERT INTO sample VALUES('before')",
	} {
		if _, err = db.Exec(stmt); err != nil {
			break
		}
	}
	if err == nil {
		_, err = db.Exec("INSERT INTO schema_meta VALUES('updates.settings',?)", string(encoded))
	}
	if err = errors.Join(err, db.Close()); err != nil {
		t.Fatal(err)
	}
	// Child files inherit DACLs, not their parent's owner. The runner's token
	// supplies the default owner (which can be its individual user even when
	// elevated). Model the enrolled installation explicitly instead of relying
	// on that machine-specific default.
	for _, file := range []struct {
		path, serviceRights string
		rights              uint32
	}{
		{filepath.Join(BinDir, "loom.exe"), "FRFX", windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE},
		{filepath.Join(ConfigDir, "loom.yaml"), "0x1301bf", 0x1301bf},
		{nestedConfig, "0x1301bf", 0x1301bf},
		{DBPath, "0x1301bf", 0x1301bf},
	} {
		if err := winProtect(file.path, winSDDL(file.serviceRights, false)); err != nil {
			t.Fatalf("secure fixture file %s: %v", filepath.Base(file.path), err)
		}
		if err := winTrusted(file.path, file.rights, false); err != nil {
			t.Fatalf("validate fixture file %s: %v", filepath.Base(file.path), err)
		}
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
			if fail && !errors.Is(err, errWinTestHealth) {
				t.Fatalf("transaction failed before simulated candidate health check: %v", err)
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
			nested, err := os.ReadFile(filepath.Join(ConfigDir, "nested", "settings"))
			if err != nil || string(nested) != "preserved nested configuration" {
				t.Fatalf("nested configuration=%q %v", nested, err)
			}
			winTestStagingDirectories(t)
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
	if err := winCreateDir(work, winSDDL("", true), 0, true); err != nil {
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
	winTestStagingDirectories(t)
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

func TestWindowsNTFSRecoveryFinishesCommittedCandidate(t *testing.T) {
	tx, c := winTestTransaction(t)
	work := filepath.Join(StateDir, "transaction")
	if err := winCreateDir(work, winSDDL("", true), 0, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "new-binary"), []byte("new executable"), 0600); err != nil {
		t.Fatal(err)
	}
	c.stopped = true
	target := Identity{"1.1.0", strings.Repeat("b", 40), "refs/tags/v1.1.0", "stable"}
	j, err := tx.snapshot(t.Context(), state{Installed: c.r.Identity}, target, work)
	if err != nil {
		t.Fatal(err)
	}
	if err := winReplace(filepath.Join(BinDir, "loom.exe"), filepath.Join(work, "new-binary"), j.Binary); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", DBPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("UPDATE sample SET value = ?", "candidate write")
	if err = errors.Join(err, db.Close()); err != nil {
		t.Fatal(err)
	}
	j.Committed = true
	if err := winAtomicJSON(StateDir, "journal.json", j); err != nil {
		t.Fatal(err)
	}
	if err := tx.recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(BinDir, "loom.exe"))
	if err != nil || string(got) != "new executable" {
		t.Fatalf("committed binary rolled back: %q, %v", got, err)
	}
	db, err = sql.Open("sqlite", DBPath)
	if err != nil {
		t.Fatal(err)
	}
	var value string
	err = errors.Join(db.QueryRow("SELECT value FROM sample").Scan(&value), db.Close())
	if err != nil || value != "candidate write" {
		t.Fatalf("committed database rolled back: %q, %v", value, err)
	}
	var saved state
	if err := readJSON(filepath.Join(StateDir, "state.json"), &saved); err != nil || !sameIdentity(saved.Installed, target) {
		t.Fatalf("committed identity not recorded: %+v, %v", saved, err)
	}
}

func TestWindowsNTFSRecoveryDoesNotReplayAfterServiceRestart(t *testing.T) {
	tx, c := winTestTransaction(t)
	c.failHealth = true
	c.failRollbackHealth = true
	if err := tx.run(t.Context()); !errors.Is(err, errWinTestHealth) {
		t.Fatalf("expected failed candidate and interrupted recovery, got %v", err)
	}
	var j winJournal
	if err := readJSON(filepath.Join(StateDir, "journal.json"), &j); err != nil || !j.Restored {
		t.Fatalf("rollback completion not recorded: %+v, %v", j, err)
	}
	// The old service is running despite its health check failing. Its writes
	// must survive a later recovery attempt or helper restart.
	db, err := sql.Open("sqlite", DBPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("UPDATE sample SET value = ?", "after restart")
	if err = errors.Join(err, db.Close()); err != nil {
		t.Fatal(err)
	}
	if err := tx.recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", DBPath)
	if err != nil {
		t.Fatal(err)
	}
	var value string
	err = errors.Join(db.QueryRow("SELECT value FROM sample").Scan(&value), db.Close())
	if err != nil || value != "after restart" {
		t.Fatalf("recovery overwrote post-rollback writes: %q, %v", value, err)
	}
}

func TestWindowsNTFSStopFailureResumesOldService(t *testing.T) {
	tx, c := winTestTransaction(t)
	c.failStop = true
	if err := tx.run(t.Context()); err == nil {
		t.Fatal("stop failure was ignored")
	}
	if c.stopped || c.starts != 1 {
		t.Fatal("old service was not resumed after stop failure")
	}
	got, err := os.ReadFile(filepath.Join(BinDir, "loom.exe"))
	if err != nil || string(got) != "old executable" {
		t.Fatalf("old executable changed: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(StateDir, "journal.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected journal after stop failure: %v", err)
	}
}

func TestWindowsNTFSVerificationFailureDoesNotStopService(t *testing.T) {
	tx, c := winTestTransaction(t)
	pub := tx.publisher.(*winTestPublisher)
	pub.downloadErr = errors.New("simulated digest verification failure")
	if err := tx.run(t.Context()); !errors.Is(err, pub.downloadErr) {
		t.Fatalf("verification failure not propagated: %v", err)
	}
	if c.stopped || c.starts != 0 {
		t.Fatal("service changed after an unverified download")
	}
	got, err := os.ReadFile(filepath.Join(BinDir, "loom.exe"))
	if err != nil || string(got) != "old executable" {
		t.Fatalf("old executable changed: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(StateDir, "transaction", "new-binary")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unverified archive was extracted: %v", err)
	}
}

func TestWindowsNTFSRequiresOptInBeforeDownload(t *testing.T) {
	tx, c := winTestTransaction(t)
	c.r.Settings.AutoInstall = false
	if err := tx.run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if tx.publisher.(*winTestPublisher).downloads != 0 || c.stopped {
		t.Fatal("update proceeded without explicit opt-in")
	}
}

func winTestStagingDirectories(t *testing.T) {
	t.Helper()
	work := filepath.Join(StateDir, "transaction")
	for _, path := range []string{work, filepath.Join(work, "config"), filepath.Join(work, "config", "nested")} {
		if err := winTrusted(path, 0, true); err != nil {
			t.Fatalf("untrusted staging directory %s: %v", path, err)
		}
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, winReadSD)
		if err != nil {
			t.Fatal(err)
		}
		owner, _, err := sd.Owner()
		if err != nil {
			t.Fatal(err)
		}
		if owner.String() != "S-1-5-18" {
			t.Fatalf("staging directory %s did not get its explicit SYSTEM owner: %s", path, owner.String())
		}
	}
}

func TestWindowsNTFSRejectsUntrustedStagingAncestor(t *testing.T) {
	_, _ = winTestTransaction(t)
	work := filepath.Join(StateDir, "transaction")
	if err := winCreateDir(work, winSDDL("", true), 0, true); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(work, "new-binary")
	if err := os.WriteFile(source, []byte("new executable"), 0600); err != nil {
		t.Fatal(err)
	}
	untrusted := strings.Replace(winSDDL("", true), "O:SY", "O:BU", 1)
	if err := winProtect(work, untrusted); err != nil {
		t.Fatal(err)
	}
	if f, err := winOpen(source, 0); err == nil {
		f.Close()
		t.Fatal("untrusted staging ancestor accepted")
	} else if !strings.Contains(err.Error(), "native Windows installation ancestor is not system-owned") {
		t.Fatalf("unexpected ancestor rejection: %v", err)
	}
	if err := winCreateDir(work, winSDDL("", true), 0, true); err == nil {
		t.Fatal("existing untrusted staging directory accepted")
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
	if err := winProtect(file, winSDDL("0x1301bf", false)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file+":private", []byte("stream"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := winCopySnapshot(file, filepath.Join(StateDir, "invalid-copy"), "with-stream", 1024); err == nil ||
		(!strings.Contains(err.Error(), "NTFS alternate streams require manual updates") && !strings.Contains(err.Error(), "unexpected NTFS stream")) {
		t.Fatalf("snapshot did not reject the alternate stream: %v", err)
	}
}

func TestWindowsNTFSSnapshotRejectsUntrustedOwner(t *testing.T) {
	_, _ = winTestTransaction(t)
	source := filepath.Join(BinDir, "loom.exe")
	dest := filepath.Join(StateDir, "owner-test-copy")
	// Force an untrusted owner regardless of the runner token's default owner.
	// Keep the DACL unchanged to isolate the ownership check.
	untrusted := strings.Replace(winSDDL("FRFX", false), "O:SY", "O:BU", 1)
	if err := winProtect(source, untrusted); err != nil {
		t.Fatal(err)
	}
	if _, err := winCopySnapshot(source, dest, "loom.exe", 1024); err == nil ||
		!strings.Contains(err.Error(), `Windows snapshot metadata "loom.exe": validate Windows file descriptor: untrusted Windows object owner`) {
		t.Fatalf("snapshot did not report the underlying owner rejection: %v", err)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("untrusted snapshot created a destination: %v", err)
	}
	if err := winProtect(source, winSDDL("FRFX", false)); err != nil {
		t.Fatal(err)
	}
	if _, err := winCopySnapshot(source, dest, "loom.exe", 1024); err != nil {
		t.Fatalf("system-owned snapshot rejected: %v", err)
	}
}
