//go:build darwin

package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Hashes pin the canonical shipped plists, not mutable launchctl output. Tests
// require packaging and these expected hashes to change together.
var macPlists = map[string]string{
	macPlist:        "e38e1e5c10e80c7d678b6fa44ca859e8576b9ba9f179c8d7bb32045c47b837a4",
	macUpdaterPlist: "512654ac549f3e5ad70188ebf55be658774e6a19b3690bbe964c7ca42784aee7",
}

func checkMacPlist(path, want string) error {
	if err := trusted(path, 0644, false); err != nil {
		return err
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- fixed canonical plist table keys
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != want {
		return errors.New("native updater refuses customized launch daemons")
	}
	return nil
}

func macCommand(ctx context.Context, exe string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 130*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...) // #nosec G204 -- private call sites select only /bin/launchctl and /usr/sbin/lsof with fixed operations
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL=C"}
	out := cappedOutput{limit: 1 << 20}
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, errors.New("fixed native service operation failed")
	}
	return out.Bytes(), nil
}

type launchd struct {
	token     string
	gate      *startGate
	enrolling bool
}

func (s *launchd) validate(ctx context.Context) error {
	if err := nativeInstalled(); err != nil {
		return err
	}
	if !s.enrolling {
		if err := enrollmentStatus(); err != nil {
			return err
		}
	}
	if err := trusted(StateDir, 0700, true); err != nil {
		return err
	}
	uid, gid, err := macAccount()
	if err != nil {
		return err
	}
	for _, p := range []string{ConfigDir, DataDir} {
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		m, _, err := metadata(p, info)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0750 || m.UID != int(uid) || m.GID != int(gid) {
			return errors.New("native config/data ownership or paths changed")
		}
		f, err := os.Open(p) // #nosec G304 -- fixed ConfigDir/DataDir only, verified non-symlink above
		if err != nil {
			return err
		}
		err = noACL(f)
		_ = f.Close()
		if err != nil {
			return err
		}
	}
	for _, p := range []string{"/bin/launchctl", "/usr/sbin/lsof"} {
		if err := trusted(p, 0755, false); err != nil {
			return err
		}
	}
	init, err := unix.SysctlKinfoProc("kern.proc.pid", 1)
	if err != nil || strings.TrimRight(string(init.Proc.P_comm[:]), "\x00") != "launchd" {
		return errors.New("native updates require system launchd")
	}
	if raw, err := macCommand(ctx, "/bin/launchctl", "print", macService); err == nil {
		return validateMacJob(raw)
	}
	// A stopped/unloaded job is valid during recovery. No database restoration
	// occurs until stop/quiescent also proves the dedicated UID has no processes.
	return nil
}

func validateMacJob(raw []byte) error {
	fields, args, err := macJobFields(raw)
	if err != nil {
		return err
	}
	for key, want := range map[string]string{"path": macPlist, "program": BinDir + "/loom", "username": "_loom", "working directory": DataDir} {
		if fields[key] != want {
			return errors.New("loaded launch daemon differs from enrolled service")
		}
	}
	if !slices.Equal(args, []string{BinDir + "/loom", "serve", "--config", ConfigDir + "/loom.yaml"}) {
		return errors.New("loaded service arguments differ from enrollment")
	}
	return nil
}

func (s *launchd) runtime(ctx context.Context) (Runtime, error) {
	var r Runtime
	client := http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:1925"+RuntimePath, nil)
	if err != nil {
		return r, err
	}
	req.Header.Set("X-Loom-Updater-Token", s.token)
	resp, err := client.Do(req)
	if err != nil {
		return r, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return r, errors.New("native runtime authentication failed")
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&r)
	return r, err
}

func (s *launchd) quiescent(context.Context) error {
	uid, _, err := macAccount()
	if err != nil {
		return err
	}
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.uid", int(uid))
	if err != nil {
		return err
	}
	if len(procs) != 0 {
		return errors.New("the dedicated _loom identity still has processes; refusing snapshot/restore")
	}
	return nil
}

func (s *launchd) stop(ctx context.Context) error {
	s.gate.allow.Store(false)
	if raw, err := macCommand(ctx, "/bin/launchctl", "print", macService); err == nil {
		if err := validateMacJob(raw); err != nil {
			return err
		}
		if _, err := macCommand(ctx, "/bin/launchctl", "bootout", macService); err != nil {
			return err
		}
	}
	deadline := time.NewTimer(125 * time.Second)
	defer deadline.Stop()
	for {
		if err := s.quiescent(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("launchd did not fully quiesce the service identity")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (s *launchd) start(ctx context.Context) error {
	s.gate.allow.Store(true)
	if raw, err := macCommand(ctx, "/bin/launchctl", "print", macService); err == nil {
		return validateMacJob(raw)
	}
	_, err := macCommand(ctx, "/bin/launchctl", "bootstrap", "system", macPlist)
	if err != nil {
		s.gate.allow.Store(false)
	}
	return err
}

func (s *launchd) healthy(ctx context.Context, want Identity) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for {
		r, err := s.runtime(ctx)
		if err == nil && r.Native && r.Ready && r.Engine == "sqlite" && sameIdentity(r.Identity, want) && s.processMatches(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("launchd service failed authenticated version-specific health")
		case <-time.After(time.Second):
		}
	}
}

func (s *launchd) processMatches(ctx context.Context) bool {
	raw, err := macCommand(ctx, "/bin/launchctl", "print", macService)
	if err != nil || validateMacJob(raw) != nil {
		return false
	}
	fields, _, err := macJobFields(raw)
	if err != nil {
		return false
	}
	pid, err := strconv.ParseInt(fields["pid"], 10, 32)
	if err != nil || pid <= 0 {
		return false
	}
	info, err := unix.SysctlKinfoProc("kern.proc.pid", int(pid))
	if err != nil {
		return false
	}
	uid, gid, err := macAccount()
	return err == nil && info.Eproc.Ucred.Uid == uid && info.Eproc.Ucred.Groups[0] == gid &&
		macProcessImage(ctx, int(pid), BinDir+"/loom")
}

func macProcessImage(ctx context.Context, pid int, path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Dev < 0 || pid <= 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := macCommand(ctx, "/usr/sbin/lsof", "-a", "-p", strconv.Itoa(pid), "-d", "txt", "-F", "Dinf")
	if err != nil {
		return false
	}
	wantDev := fmt.Sprintf("D0x%x", uint32(stat.Dev))
	wantInode := "i" + strconv.FormatUint(stat.Ino, 10)
	for _, record := range strings.Split(string(raw), "ftxt\n") {
		lines := strings.Split(record, "\n")
		if slices.Contains(lines, wantDev) && slices.Contains(lines, wantInode) && slices.Contains(lines, "n"+path) {
			return true
		}
	}
	return false
}
