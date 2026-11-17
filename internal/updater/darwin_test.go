//go:build darwin

package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestMacCanonicalPlists(t *testing.T) {
	for path, want := range macPlists {
		raw, err := os.ReadFile(filepath.Join("../../packaging/macos", filepath.Base(path)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != want {
			t.Fatalf("canonical plist drift: %s", path)
		}
	}
}

func TestMacACLUsesOpenInode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forged\nheader")
	if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := noACL(f); err != nil {
		t.Fatalf("plain open inode rejected: %v", err)
	}
	if out, err := exec.Command("/bin/chmod", "+a", "everyone allow read", path).CombinedOutput(); err != nil {
		t.Fatalf("create disposable ACL: %v %s", err, out)
	}
	if err := noACL(f); err == nil {
		t.Fatal("Darwin ACL was not refused")
	}
}

func TestMacCommandOutputBound(t *testing.T) {
	w := &cappedOutput{limit: 32}
	if _, err := io.Copy(w, strings.NewReader(strings.Repeat("x", 10000))); err == nil {
		t.Fatal("unbounded output accepted")
	}

	if len(w.Bytes()) > 32 {
		t.Fatal("command exceeded memory bound")
	}
}

func TestMacSecurityAttributeFailsClosed(t *testing.T) {
	plain := make([]byte, 12)
	binary.LittleEndian.PutUint32(plain[:4], 12)
	if err := noMacACLAttribute(plain); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{nil, plain[:8], bytes.Repeat([]byte{255}, 64)} {
		if err := noMacACLAttribute(bad); err == nil {
			t.Fatal("malformed security attribute accepted")
		}
	}
	path := filepath.Join(t.TempDir(), "custom")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := unix.Setxattr(path, "user.loom-test", []byte("x"), 0); err != nil {
		t.Fatal(err)
	}
	if err := noACL(f); err == nil {
		t.Fatal("xattr was silently dropped")
	}
}

func TestMacSystemPathChecksAreReadableByService(t *testing.T) {
	for _, p := range []string{"/Library", "/Library/Application Support", "/Library/LaunchDaemons"} {
		if err := trusted(p, 0755, true); err != nil {
			t.Fatalf("system ancestor check %s: %v", p, err)
		}
	}
	for _, p := range []string{"/bin/launchctl", "/usr/sbin/lsof"} {
		if err := trusted(p, 0755, false); err != nil {
			t.Fatalf("system tool check %s: %v", p, err)
		}
	}
}

func macTestGate(t *testing.T, uid uint32) (*startGate, string) {
	t.Helper()
	// Darwin's sockaddr_un is limited to 104 bytes; t.TempDir can exceed it.
	dir, err := os.MkdirTemp("/tmp", "loom-gate-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	g := &startGate{}
	go g.serve(l, uid)
	return g, path
}

func gateRequest(t *testing.T, path, request string) string {
	t.Helper()
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(c, request); err != nil {
		t.Fatal(err)
	}
	if err := c.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(c)
	return string(raw)
}

func TestMacGateIsReadOnlyAndPeerAuthenticated(t *testing.T) {
	g, path := macTestGate(t, uint32(os.Geteuid()))
	if got := gateRequest(t, path, "START\n"); got != "WAIT\n" {
		t.Fatalf("startup gate initially open: %q", got)
	}
	g.allow.Store(true)
	if got := gateRequest(t, path, "START\n"); got != "GO\n" {
		t.Fatalf("authorized startup refused: %q", got)
	}
	for _, bad := range []string{"enroll\n", "START\n/evil", "START\nSTART\n", "", "START"} {
		if got := gateRequest(t, path, bad); got != "" {
			t.Fatalf("invalid IPC request accepted: %q => %q", bad, got)
		}
	}
	_, wrongPeer := macTestGate(t, uint32(os.Geteuid())+1)
	if got := gateRequest(t, wrongPeer, "START\n"); got != "" {
		t.Fatal("wrong peer identity accepted")
	}
	if os.Geteuid() != 0 {
		if err := queryMacGate(t.Context(), path); err == nil {
			t.Fatal("startup client accepted a non-root helper")
		}
	}
}

func TestMacJobValidation(t *testing.T) {
	job := "system/dev.loom.server = {\n\tpath = " + macPlist + "\n\tprogram = " + BinDir + "/loom\n\targuments = {\n\t\t" + BinDir + "/loom\n\t\tserve\n\t\t--config\n\t\t" + ConfigDir + "/loom.yaml\n\t}\n\tusername = _loom\n\tworking directory = " + DataDir + "\n\tpid = 123\n\tresources = {\n\t\tpid = 666\n\t}\n}\n"
	if err := validateMacJob([]byte(job)); err != nil {
		t.Fatal(err)
	}
	fields, _, err := macJobFields([]byte(job))
	if err != nil || fields["pid"] != "123" {
		t.Fatal("nested launchctl fields overrode identity")
	}
	for _, bad := range []string{
		strings.Replace(job, "username = _loom", "username = root", 1),
		strings.Replace(job, "\t\tserve", "\t\tarbitrary", 1),
		strings.Replace(job, "\tpath = "+macPlist, "\t\tpath = "+macPlist, 1),
		strings.Replace(job, "\tpid = 123", "\tpid = 123\n\tpid = 666", 1),
		strings.Replace(job, "\t\t--config", "\t\t\t--config", 1),
	} {
		if err := validateMacJob([]byte(bad)); err == nil {
			t.Fatal("customized launch daemon accepted")
		}
	}
}

func TestMacSnapshotIncludesUncheckpointedWALWithSpaces(t *testing.T) {
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
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; CREATE TABLE payload(value TEXT); INSERT INTO payload VALUES ('committed-wal')"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source + "-wal")
	if err != nil || info.Size() == 0 {
		t.Fatal("test requires committed WAL")
	}
	var stream bytes.Buffer
	if err := vacuumToStream(t.Context(), source, filepath.Join(dir, "snapshot 'quoted'.db"), &stream); err != nil {
		t.Fatal(err)
	}
	restore := filepath.Join(dir, "restore.db")
	if err := os.WriteFile(restore, stream.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	check, err := sql.Open("sqlite", restore)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var got string
	if err := check.QueryRow("SELECT value FROM payload").Scan(&got); err != nil || got != "committed-wal" {
		t.Fatalf("inconsistent standalone snapshot: %q %v", got, err)
	}
	w := &snapshotWriter{out: io.Discard, remaining: 1}
	if _, err := w.Write([]byte("too large")); err == nil {
		t.Fatal("snapshot bound not enforced")
	}
}

func TestMacProcessImageUsesDeviceAndInode(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		t.Fatal(err)
	}
	if !macProcessImage(ctx, os.Getpid(), self) {
		t.Fatal("current process image not recognized for PID " + strconv.Itoa(os.Getpid()))
	}
	if macProcessImage(ctx, os.Getpid(), "/bin/ls") {
		t.Fatal("wrong executable image accepted")
	}
}

func TestMacUnmanagedInstallRemainsIneligible(t *testing.T) {
	if ok, _ := Capability(Binding{ConfigDir, DataDir, DBPath, "sqlite", "127.0.0.1:1925"}); ok {
		t.Fatal("unmanaged test executable is eligible")
	}

	if ok, _ := Capability(Binding{Engine: "postgres"}); ok {
		t.Fatal("PostgreSQL automatic installation enabled")
	}
	if err := StartupGuard(t.Context()); err != nil {
		t.Fatalf("unmanaged developer startup changed: %v", err)
	}
	if err := Main(t.Context(), []string{"arbitrary", "/tmp/evil"}); err == nil {
		t.Fatal("arbitrary helper arguments accepted")
	}
}

func TestMacDirectoryServicesAndUIDProcessEnumeration(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	account, err := user.Lookup(current.Username)
	if err != nil || account.Uid != strconv.Itoa(os.Getuid()) {
		t.Fatal("native account lookup unavailable")
	}
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.uid", os.Geteuid())
	if err != nil {
		t.Fatal(err)
	}
	for _, proc := range procs {
		if int(proc.Proc.P_pid) == os.Getpid() {
			return
		}
	}
	t.Fatal("UID process enumeration missed this process; quiescence cannot be trusted")
}
