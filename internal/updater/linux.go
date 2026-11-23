//go:build linux

package updater

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/ebenderooock/loom/internal/updates"
)

var unitFiles = map[string]string{
	"/etc/systemd/system/loom.service":                 mainUnit,
	"/etc/systemd/system/loom-updater.service":         updaterUnit,
	"/etc/systemd/system/loom-updater-recover.service": recoverUnit,
	"/etc/systemd/system/loom-updater.timer":           timerUnit,
	"/etc/systemd/system/loom.service.d/updater.conf":  dependencyUnit,
}

func noACL(f *os.File) error {
	n, err := unix.Flistxattr(int(f.Fd()), nil)
	if errors.Is(err, unix.ENOTSUP) {
		return nil
	}
	if err != nil {
		return err
	}
	if n != 0 {
		return errors.New("native updater refuses extended attributes/ACLs; use manual updates")
	}
	return nil
}

func trusted(path string, mode os.FileMode, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("native updater enrollment missing or invalid: %w", err)
	}
	meta, links, err := metadata(path, info)
	if err != nil || meta.UID != 0 || meta.GID != 0 || info.Mode().Perm() != mode ||
		info.IsDir() != directory || (!directory && (!info.Mode().IsRegular() || links != 1)) ||
		info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return errors.New("native updater requires root-owned, non-writable canonical install paths")
	}
	f, err := os.Open(path) // #nosec G304 -- only fixed installer paths reach this private ownership check
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return noACL(f)
}

func nativeInstalled() error {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" && runtime.GOARCH != "arm" {
		return errors.New("unsupported native updater architecture")
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return errors.New("containers use image updates")
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return errors.New("kubernetes uses image updates")
	}
	self, err := os.Executable()
	if err != nil || (self != BinDir+"/loom" && self != BinDir+"/loom-updater") {
		return errors.New("automatic installation requires the installer-managed executable")
	}
	for _, p := range []string{"/usr", "/usr/local", "/usr/local/lib", BinDir, "/etc", "/etc/systemd", "/etc/systemd/system", "/etc/systemd/system/loom.service.d"} {
		if err := trusted(p, 0755, true); err != nil {
			return err
		}
	}
	for _, p := range []string{BinDir + "/loom", BinDir + "/loom-updater"} {
		if err := trusted(p, 0755, false); err != nil {
			return err
		}
	}
	for p, expected := range unitFiles {
		if err := trusted(p, 0644, false); err != nil {
			return err
		}
		raw, err := os.ReadFile(p) // #nosec G304 -- keys of the fixed unitFiles table
		if err != nil || string(raw) != expected {
			return errors.New("native updater refuses customized service units")
		}
	}
	if err := trusted(markerPath, 0644, false); err != nil {
		return err
	}
	raw, err := os.ReadFile(markerPath)
	if err != nil || string(raw) != "loom-systemd-updater-v1\n" {
		return errors.New("invalid native updater enrollment")
	}
	if _, err := readToken(); err != nil {
		return err
	}
	// A container without Docker/Kubernetes markers must still have real
	// systemd as PID 1; never infer eligibility solely from writable files.
	comm, err := os.ReadFile("/proc/1/comm")
	if err != nil || strings.TrimSpace(string(comm)) != "systemd" {
		return errors.New("native updates require systemd as PID 1")
	}
	return nil
}

type systemd struct {
	token     string
	enrolling bool
}

func systemctl(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "/usr/bin/systemctl", args...) // #nosec G204 -- private call sites pass only literal operations and loom.service; never runtime/request data
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "SYSTEMD_PAGER="}
	raw, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("fixed Loom systemd operation failed: %w", err)
	}
	return raw, nil
}

func (s systemd) validate(ctx context.Context) error {
	if err := nativeInstalled(); err != nil {
		return err
	}
	if !s.enrolling {
		if err := enrollmentStatus(); err != nil {
			return err
		}
	}
	for _, p := range []string{"/var", "/var/lib"} {
		if err := trusted(p, 0755, true); err != nil {
			return err
		}
	}
	if err := trusted(StateDir, 0700, true); err != nil {
		return err
	}
	loom, err := user.Lookup("loom")
	if err != nil {
		return err
	}
	for _, path := range []string{ConfigDir, DataDir} {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		meta, _, err := metadata(path, info)
		if err != nil || !info.IsDir() || strconv.Itoa(meta.UID) != loom.Uid || strconv.Itoa(meta.GID) != loom.Gid ||
			info.Mode().Perm() != 0750 {
			return errors.New("native data/config directories must retain installer ownership and mode")
		}
		f, err := os.Open(path) // #nosec G304 -- literal ConfigDir and DataDir only
		if err != nil {
			return err
		}
		err = noACL(f)
		_ = f.Close()
		if err != nil {
			return err
		}
	}
	if err := trusted("/usr/bin/systemctl", 0755, false); err != nil {
		return err
	}
	raw, err := systemctl(ctx, "show", "loom.service", "--property=FragmentPath,DropInPaths,NeedDaemonReload,User,Group,KillMode,RootDirectory,Transient")
	if err != nil {
		return err
	}
	props := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			props[key] = value
		}
	}
	for key, value := range map[string]string{
		"FragmentPath":     "/etc/systemd/system/loom.service",
		"DropInPaths":      "/etc/systemd/system/loom.service.d/updater.conf",
		"NeedDaemonReload": "no", "User": "loom", "Group": "loom", "KillMode": "control-group", "RootDirectory": "", "Transient": "no",
	} {
		if actual, ok := props[key]; !ok || actual != value {
			return errors.New("loaded Loom service differs from the enrolled unit")
		}
	}
	return nil
}

func (s systemd) runtime(ctx context.Context) (Runtime, error) {
	var value Runtime
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:1925"+RuntimePath, nil)
	if err != nil {
		return value, err
	}
	req.Header.Set("X-Loom-Updater-Token", s.token)
	resp, err := client.Do(req)
	if err != nil {
		return value, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return value, errors.New("native runtime authentication/readiness failed")
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&value)
	return value, err
}

func (s systemd) stop(ctx context.Context) error {
	if _, err := systemctl(ctx, "stop", "loom.service"); err != nil {
		return err
	}
	return s.quiescent(ctx)
}
func (s systemd) quiescent(ctx context.Context) error {
	raw, err := systemctl(ctx, "show", "loom.service", "--property=ActiveState,MainPID,ControlPID,SubState")
	if err != nil {
		return err
	}
	props := string(raw)
	if (!strings.Contains(props, "ActiveState=inactive\n") && !strings.Contains(props, "ActiveState=activating\n")) ||
		!strings.Contains(props, "MainPID=0\n") || !strings.Contains(props, "ControlPID=0\n") {
		return errors.New("loom service did not fully quiesce")
	}
	return nil
}
func (s systemd) start(ctx context.Context) error {
	_, err := systemctl(ctx, "start", "loom.service")
	return err
}
func (s systemd) healthy(ctx context.Context, want Identity) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		value, err := s.runtime(ctx)
		if err == nil && value.Native && value.Ready && value.Engine == "sqlite" && sameIdentity(value.Identity, want) && s.processMatches(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("updated Loom failed authenticated version-specific health")
		case <-tick.C:
		}
	}
}
func (s systemd) processMatches(ctx context.Context) bool {
	raw, err := systemctl(ctx, "show", "loom.service", "--property=MainPID", "--value")
	pid, parseErr := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 32)
	if err != nil || parseErr != nil || pid == 0 {
		return false
	}
	running, err := os.Stat(fmt.Sprintf("/proc/%d/exe", pid))
	installed, statErr := os.Stat(BinDir + "/loom")
	return err == nil && statErr == nil && os.SameFile(running, installed)
}

// Main accepts only three fixed operations, all root-only. There is no setuid,
// sudoers rule, service-account IPC command, URL/path flag or shell evaluator.
func Main(ctx context.Context, args []string) error {
	if len(args) == 1 && args[0] == "snapshot" {
		return snapshotChild(ctx)
	}
	if os.Geteuid() != 0 {
		return errors.New("loom-updater requires root under its installed systemd unit")
	}
	if len(args) > 1 || (len(args) == 1 && args[0] != "enroll" && args[0] != "recover" && args[0] != "preflight") {
		return errors.New("only fixed enroll/recover/preflight operations are supported")
	}
	operation := ""
	if len(args) == 1 {
		operation = args[0]
	}
	if operation == "preflight" {
		return preflight()
	}
	if err := prepareStateDir(); err != nil {
		return err
	}
	lock, err := os.OpenFile(StateDir+"/lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if operation == "recover" && errors.Is(err, unix.EWOULDBLOCK) {
			return nil
		} // active updater owns recovery
		return errors.New("another native update is active")
	}
	if operation == "enroll" {
		return enroll(ctx)
	}
	if operation == "recover" {
		if _, err := os.Stat(StateDir + "/journal.json"); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
	}
	token, err := readToken()
	if err != nil {
		return err
	}
	service := systemd{token: token}
	e := engine{paths: layout{BinDir, ConfigDir, DataDir, StateDir}, platform: nativePlatform(), service: service, publisher: &checkedPublisher{Checker: updates.NewChecker("dev"), stateDir: StateDir}}
	if operation == "recover" {
		return e.recover(ctx, false)
	}
	return e.run(ctx)
}

type boundedWriter struct {
	out       io.Writer
	remaining int64
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errors.New("database snapshot exceeds 64 GiB")
	}
	n, err := w.out.Write(p)
	w.remaining -= int64(n)
	return n, err
}

func (s systemd) snapshotDB(ctx context.Context, dest string) error {
	account, err := user.Lookup("loom")
	if err != nil {
		return err
	}
	uid, err := strconv.ParseUint(account.Uid, 10, 32)
	if err != nil {
		return err
	}
	gid, err := strconv.ParseUint(account.Gid, 10, 32)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) // #nosec G304 -- fixed root-private snapshot destination
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, BinDir+"/loom-updater", "snapshot")
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C"}
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: []uint32{uint32(gid)}}}
	command.Stdout = &boundedWriter{out: out, remaining: 64 << 30}
	// Never include database contents, tokens, or source-controlled stderr in
	// updater logs. Failure is sufficient to abort before executable changes.
	err = command.Run()
	return errors.Join(err, out.Sync(), out.Close())
}

func snapshotChild(ctx context.Context) error {
	account, err := user.Lookup("loom")
	if err != nil {
		return err
	}
	if os.Geteuid() == 0 || strconv.Itoa(os.Geteuid()) != account.Uid {
		return errors.New("snapshot requires the unprivileged loom account")
	}
	dir, err := os.MkdirTemp(DataDir, ".loom-snapshot-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "snapshot.db")
	db, err := sql.Open("sqlite", "file:"+DBPath+"?mode=ro")
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return err
	}
	f, err := os.Open(path) // #nosec G304 -- private temporary snapshot under the fixed DataDir, unprivileged process
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(&boundedWriter{out: os.Stdout, remaining: 64 << 30}, f)
	return err
}

func nativePlatform() string {
	if runtime.GOARCH == "arm" {
		return "linux-armv7"
	}
	return "linux-" + runtime.GOARCH
}
func prepareStateDir() error {
	if err := trusted("/var/lib", 0755, true); err != nil {
		return err
	}
	if err := os.Mkdir(StateDir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return trusted(StateDir, 0700, true)
}
func readToken() (string, error) {
	info, err := os.Lstat(tokenPath)
	if err != nil {
		return "", err
	}
	meta, links, err := metadata(tokenPath, info)
	group, groupErr := user.LookupGroup("loom")
	if err != nil || groupErr != nil || meta.UID != 0 || strconv.Itoa(meta.GID) != group.Gid || links != 1 ||
		!info.Mode().IsRegular() || info.Mode().Perm() != 0640 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return "", errors.New("invalid updater authentication token ownership")
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
func enroll(ctx context.Context) error {
	// Root explicitly opts into installing the mechanism; administrator consent
	// to actual automatic installation is still false in the application.
	if _, err := os.Lstat(tokenPath); errors.Is(err, os.ErrNotExist) {
		group, err := user.LookupGroup("loom")
		if err != nil {
			return err
		}
		gid, err := strconv.Atoi(group.Gid)
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
		_, writeErr := f.WriteString(hex.EncodeToString(random))
		if writeErr == nil {
			writeErr = f.Chown(0, gid)
		}
		if writeErr == nil {
			writeErr = f.Chmod(0640)
		}
		err = errors.Join(writeErr, f.Sync(), f.Close())
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := writeEnrollment(markerPath, "loom-systemd-updater-v1\n"); err != nil {
		return err
	}
	if err := syncDir(BinDir); err != nil {
		return err
	}
	token, err := readToken()
	if err != nil {
		return err
	}
	service := systemd{token: token, enrolling: true}
	if err := service.validate(ctx); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(StateDir, "journal.json")); err == nil {
		return errors.New("recover pending transaction before enrollment")
	}
	var r Runtime
	for range 30 {
		r, err = service.runtime(ctx)
		if err == nil && r.Ready {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if err != nil || !r.Ready || !r.Native || r.Engine != "sqlite" ||
		!updates.ValidSource(r.Version, r.Ref, r.SHA, r.Channel) || !service.processMatches(ctx) {
		return errors.New("enrollment requires a ready release binary with matching source metadata at the fixed native paths")
	}
	if err := atomicJSON(StateDir, "state.json", state{Installed: r.Identity}); err != nil {
		return err
	}
	if err := writeEnrollment(BinDir+"/updater-enrolled", updates.TrustFingerprint()); err != nil {
		return err
	}
	return syncDir(BinDir)
}

// writeEnrollment preserves the public proof's exact mode even with umask 0077.
// Callers supply only the two literal enrollment paths, never service input.
func writeEnrollment(path, value string) error {
	if _, err := os.Lstat(path); err == nil {
		if err := trusted(path, 0644, false); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600) // #nosec G304 -- fixed proof/marker under root-owned BinDir, validated above
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString(value)
	if writeErr == nil {
		writeErr = f.Chown(0, 0)
	}
	if writeErr == nil {
		writeErr = f.Chmod(0644)
	}
	return errors.Join(writeErr, f.Sync(), f.Close())
}

func enrollmentStatus() error {
	path := BinDir + "/updater-enrolled"
	if err := trusted(path, 0644, false); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != updates.TrustFingerprint() {
		return errors.New("native updater enrollment/public-key build mismatch; reinstall and enroll the matching helper")
	}
	return nil
}
func preflight() error {
	for _, path := range []string{"/usr", "/usr/local", "/usr/local/lib", "/etc", "/etc/systemd", "/etc/systemd/system", "/var", "/var/lib"} {
		if err := trusted(path, 0755, true); err != nil {
			return err
		}
	}
	for _, path := range []string{BinDir, BinDir + "/loom", BinDir + "/loom-updater"} {
		_, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := trusted(path, 0755, path == BinDir); err != nil {
			return err
		}
	}
	for path, mode := range map[string]os.FileMode{"/etc/systemd/system/loom.service.d": 0755, StateDir: 0700} {
		if _, err := os.Lstat(path); err == nil {
			if err := trusted(path, mode, true); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return preflightUnits()
}

func preflightUnits() error {
	for path, expected := range unitFiles {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := trusted(path, 0644, false); err != nil {
			return err
		}
		raw, err := os.ReadFile(path) // #nosec G304 -- literal unitFiles table keys
		if err != nil {
			return err
		}
		previousMain := path == "/etc/systemd/system/loom.service" && string(raw) == strings.Replace(mainUnit, "ReadWritePaths=/etc/loom /var/lib/loom\n", "", 1)
		if string(raw) != expected && !previousMain {
			return errors.New("native updater refuses to overwrite a customized Loom unit")
		}
	}
	return nil
}
