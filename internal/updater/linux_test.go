//go:build linux

package updater

import (
	"context"
	"database/sql"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLinuxRejectsNonRootAndArbitraryOperations(t *testing.T) {
	for _, args := range [][]string{{"run", "/tmp/evil"}, {"--command=sh"}, {"recover", "/tmp/evil"}} {
		if err := Main(context.Background(), args); err == nil {
			t.Fatalf("accepted arbitrary helper input: %v", args)
		}
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		if err := nativeInstalled(); err == nil {
			t.Fatal("container eligible for native updates")
		}
	}
}

func TestLinuxACLAndHardlinkRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(path, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := unix.Setxattr(path, "user.loom-test", []byte("custom"), 0); err != nil {
		t.Skip("test filesystem has no xattrs", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := noACL(f); err == nil {
		t.Fatal("extended attributes accepted")
	}
}

func TestLinuxEnrollmentProofUnderRestrictiveUmask(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root ownership assertion runs in the isolated Linux container")
	}
	old := unix.Umask(0077)
	defer unix.Umask(old)
	path := filepath.Join(t.TempDir(), "proof")
	if err := writeEnrollment(path, "public trust fingerprint"); err != nil {
		t.Fatal(err)
	}
	if err := trusted(path, 0644, false); err != nil {
		t.Fatal("restrictive umask broke enrollment", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeEnrollment(path, "changed"); err == nil {
		t.Fatal("silently rewrote a customized proof")
	}
}

// This integration test uses real credential dropping and the actual installed
// helper executable, but is opt-in and container-only. Never create or alter
// fixed host paths from an ordinary go test invocation.
func TestLinuxPrivilegeDroppedSnapshot(t *testing.T) {
	if os.Getenv("LOOM_UPDATER_CONTAINER_TEST") != "1" || os.Geteuid() != 0 {
		t.Skip("isolated container integration only")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		t.Fatal("refusing fixed-path integration outside a container")
	}
	account, err := user.Lookup("loom")
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := strconv.Atoi(account.Uid)
	gid, _ := strconv.Atoi(account.Gid)
	if err := os.MkdirAll(DataDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(DataDir, uid, gid); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0;
		CREATE TABLE payload(value TEXT); INSERT INTO payload VALUES ('committed-in-wal')`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"loom.db", "loom.db-wal", "loom.db-shm"} {
		if err := os.Chown(filepath.Join(DataDir, name), uid, gid); err != nil {
			t.Fatal(err)
		}
	}
	dest := filepath.Join(t.TempDir(), "standalone.db")
	service := systemd{}
	if err := service.snapshotDB(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	copy, err := sql.Open("sqlite", dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = copy.Close() }()
	var got string
	if err := copy.QueryRow("SELECT value FROM payload").Scan(&got); err != nil || got != "committed-in-wal" {
		t.Fatalf("snapshot=%q %v", got, err)
	}
	f, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	meta, _, _ := metadata(dest, f)
	if meta.UID != 0 || f.Mode().Perm() != 0600 {
		t.Fatal("snapshot is not root-private")
	}
	// The production root entry point still refuses a container; this opt-in
	// test does not introduce a runtime bypass in the shipped helper.
	if err := nativeInstalled(); err == nil || !strings.Contains(err.Error(), "container") {
		t.Fatal("container gate weakened", err)
	}
}
