//go:build darwin

package updater

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/ebenderooock/loom/internal/updates"
)

// Darwin ACLs are not listed as xattrs. fgetattrlist inspects the open vnode;
// notably, ls -e /dev/fd/N inspects fdescfs and misses the underlying ACL.
// x/sys has no fgetattrlist wrapper. Use the fixed Darwin descriptor syscall
// and its documented attrlist/attrreference ABI (both supported architectures
// are little endian). Unsupported kernels/filesystems fail closed.
func noACL(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return errors.New("native updater refuses special permission bits")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("native file flags unavailable")
	}
	switch f.Name() {
	case "/Library", "/Library/Application Support", "/Library/LaunchDaemons", "/bin/launchctl", "/usr/sbin/lsof":
		// Read-only system ancestors/tools are not transaction destinations.
		// Their SIP flags must not be copied to mutable transaction files.
	default:
		if stat.Flags != 0 {
			return errors.New("native updater cannot preserve custom file flags; use manual updates")
		}
	}
	n, err := unix.Flistxattr(int(f.Fd()), nil)
	if err != nil {
		return err
	}
	if n != 0 {
		return errors.New("native updater refuses extended attributes; use manual updates")
	}
	var attrs [24]byte
	binary.LittleEndian.PutUint16(attrs[:2], 5) // ATTR_BIT_MAP_COUNT
	binary.LittleEndian.PutUint32(attrs[4:8], unix.ATTR_CMN_EXTENDED_SECURITY)
	// Larger than Darwin's maximum 128-entry kauth ACL.
	var out [8192]byte
	//nolint:staticcheck // No x/sys fd ACL wrapper exists; unsupported syscall/ABI fails closed, covered by native tests.
	_, _, errno := unix.Syscall6(unix.SYS_FGETATTRLIST, f.Fd(), uintptr(unsafe.Pointer(&attrs[0])), uintptr(unsafe.Pointer(&out[0])), uintptr(len(out)), 0, 0) // #nosec G103 -- fixed ABI buffers, synchronous descriptor-only syscall; native ACL tests cover both outcomes
	runtime.KeepAlive(&attrs)
	runtime.KeepAlive(f)
	if errno != 0 {
		return errors.New("native descriptor ACL inspection unavailable")
	}
	return noMacACLAttribute(out[:])
}

func noMacACLAttribute(raw []byte) error {
	bad := errors.New("native updater refuses ACLs or unknown security metadata; use manual updates")
	if len(raw) < 12 {
		return bad
	}
	size := uint64(binary.LittleEndian.Uint32(raw[:4]))
	offset := uint64(binary.LittleEndian.Uint32(raw[4:8]))
	length := uint64(binary.LittleEndian.Uint32(raw[8:12]))
	if size < 12 || size > uint64(len(raw)) {
		return bad
	}
	if length == 0 {
		return nil
	}
	start := 4 + offset
	if start < 12 || start > size || length > size-start || length != 44 {
		return bad
	}
	sec := raw[start : start+length]
	// kauth_filesec header, absent ACL sentinel, no inheritance flags.
	if binary.LittleEndian.Uint32(sec[:4]) != 0x012cc16d ||
		binary.LittleEndian.Uint32(sec[36:40]) != 0xffffffff ||
		binary.LittleEndian.Uint32(sec[40:44]) != 0 {
		return bad
	}
	return nil
}

type cappedOutput struct {
	buf   bytes.Buffer
	limit int
}

func (w *cappedOutput) String() string { return w.buf.String() }
func (w *cappedOutput) Bytes() []byte  { return w.buf.Bytes() }
func (w *cappedOutput) Write(p []byte) (int, error) {
	if len(p) > w.limit-w.buf.Len() {
		return 0, errors.New("native command output exceeds limit")
	}
	return w.buf.Write(p)
}

func trusted(path string, mode os.FileMode, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	meta, links, err := metadata(path, info)
	// Ancestors such as Application Support may be root:admin, but no group
	// or other write access is accepted. Never repair/strip an existing ACL.
	if err != nil || meta.UID != 0 || info.Mode().Perm() != mode || info.IsDir() != directory ||
		(!directory && (!info.Mode().IsRegular() || links != 1)) ||
		info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return errors.New("native updater requires protected root-owned canonical paths")
	}
	f, err := os.Open(path) // #nosec G304 -- fixed native paths at private call sites
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return noACL(f)
}

func macAccount() (uint32, uint32, error) {
	u, err := user.Lookup("_loom")
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return 0, 0, err
	}
	group, err := user.LookupGroup("_loom")
	if err != nil || uid == 0 || gid == 0 || u.HomeDir != DataDir || group.Gid != u.Gid {
		return 0, 0, errors.New("incompatible dedicated _loom account")
	}
	return uint32(uid), uint32(gid), nil
}

func macPreflight() error {
	for _, p := range []string{"/Library", "/Library/Application Support", "/Library/LaunchDaemons"} {
		if err := trusted(p, 0755, true); err != nil {
			return err
		}
	}
	for p, mode := range map[string]os.FileMode{macBase: 0755, BinDir: 0755, StateDir: 0700} {
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := trusted(p, mode, true); err != nil {
			return err
		}
	}
	for _, p := range []string{BinDir + "/loom", BinDir + "/loom-updater"} {
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := trusted(p, 0755, false); err != nil {
			return err
		}
	}
	for p, want := range macPlists {
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := checkMacPlist(p, want); err != nil {
			return err
		}
	}
	return nil
}

func nativeInstalled() error {
	if runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64" {
		return errors.New("unsupported macOS architecture")
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return errors.New("kubernetes uses orchestration updates")
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return errors.New("containers use image updates")
	}
	self, err := os.Executable()
	if err != nil || (self != BinDir+"/loom" && self != BinDir+"/loom-updater") {
		return errors.New("automatic installation requires the installer-managed macOS executable")
	}
	if err := macPreflightPublic(); err != nil {
		return err
	}
	for p, want := range macPlists {
		if err := checkMacPlist(p, want); err != nil {
			return err
		}
	}
	raw, err := os.ReadFile(markerPath)
	if err != nil || string(raw) != "loom-launchd-updater-v1\n" {
		return errors.New("macOS updater enrollment marker missing")
	}
	if err := trusted(markerPath, 0644, false); err != nil {
		return err
	}
	_, err = readToken()
	return err
}

// The service may inspect public enrollment but cannot enter the private state.
func macPreflightPublic() error {
	for _, p := range []string{"/Library", "/Library/Application Support", macBase, BinDir, "/Library/LaunchDaemons"} {
		if err := trusted(p, 0755, true); err != nil {
			return err
		}
	}
	for _, p := range []string{BinDir + "/loom", BinDir + "/loom-updater"} {
		if err := trusted(p, 0755, false); err != nil {
			return err
		}
	}
	return nil
}

func enrollmentStatus() error {
	p := BinDir + "/updater-enrolled"
	if err := trusted(p, 0644, false); err != nil {
		return err
	}
	raw, err := os.ReadFile(p)
	if err != nil || string(raw) != updates.TrustFingerprint() {
		return errors.New("matching helper/public-key enrollment required")
	}
	return nil
}

func readToken() (string, error) {
	_, gid, err := macAccount()
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(tokenPath)
	if err != nil {
		return "", err
	}
	meta, links, err := metadata(tokenPath, info)
	if err != nil || meta.UID != 0 || meta.GID != int(gid) || links != 1 || info.Mode() != 0640 {
		return "", errors.New("invalid native token protection")
	}
	f, err := os.Open(tokenPath)
	if err != nil {
		return "", err
	}
	err = noACL(f)
	_ = f.Close()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(tokenPath)
	if err != nil || len(raw) != 64 {
		return "", errors.New("invalid updater authentication token")
	}
	if _, err := hex.DecodeString(string(raw)); err != nil {
		return "", err
	}
	return string(raw), nil
}

func writeMacProof(path, value string) error {
	if _, err := os.Lstat(path); err == nil {
		if err := trusted(path, 0644, false); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600) // #nosec G304 -- fixed enrollment paths only
	if err != nil {
		return err
	}
	_, err = f.WriteString(value)
	if err == nil {
		err = f.Chown(0, 0)
	}
	if err == nil {
		err = f.Chmod(0644)
	}
	return errors.Join(err, f.Sync(), f.Close(), syncDir(BinDir))
}

func macPrepare() error {
	if err := trusted(macBase, 0755, true); err != nil {
		return err
	}
	if err := os.Mkdir(StateDir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := trusted(StateDir, 0700, true); err != nil {
		return err
	}
	// Persist the private state's parent entry before any future journal can
	// claim durability (the installer may have just created this directory).
	if err := syncDir(macBase); err != nil {
		return err
	}
	if _, err := os.Lstat(tokenPath); errors.Is(err, os.ErrNotExist) {
		_, gid, err := macAccount()
		if err != nil {
			return err
		}
		random := make([]byte, 32)
		if _, err := rand.Read(random); err != nil {
			return err
		}
		f, err := os.OpenFile(tokenPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = f.WriteString(hex.EncodeToString(random))
		if err == nil {
			err = f.Chown(0, int(gid))
		}
		if err == nil {
			err = f.Chmod(0640)
		}
		if err := errors.Join(err, f.Sync(), f.Close()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return writeMacProof(markerPath, "loom-launchd-updater-v1\n")
}

func (s *launchd) snapshotDB(ctx context.Context, dest string) error {
	uid, gid, err := macAccount()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) // #nosec G304 -- root-private transaction snapshot
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, BinDir+"/loom-updater", "snapshot")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: gid, Groups: []uint32{gid}}}
	cmd.Stdout = &snapshotWriter{out: f, remaining: 64 << 30}
	return errors.Join(cmd.Run(), f.Sync(), f.Close())
}

type snapshotWriter struct {
	out       io.Writer
	remaining int64
}

func (w *snapshotWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errors.New("database snapshot exceeds 64 GiB")
	}
	n, err := w.out.Write(p)
	w.remaining -= int64(n)
	return n, err
}

func macSnapshotChild(ctx context.Context) error {
	uid, _, err := macAccount()
	if err != nil {
		return err
	}
	if os.Geteuid() == 0 || os.Geteuid() != int(uid) {
		return errors.New("snapshot requires the unprivileged _loom account")
	}
	dir, err := os.MkdirTemp(DataDir, ".loom-snapshot-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	return vacuumToStream(ctx, DBPath, filepath.Join(dir, "snapshot.db"), os.Stdout)
}
