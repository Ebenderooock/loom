//go:build windows

package updater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/ebenderooock/loom/internal/updates"
)

func nativeInstalled() error {
	if !windowsCandidate {
		return errors.New("native Windows updater production enrollment awaits NTFS/SCM acceptance; use the manual installer")
	}
	if winPathErr != nil {
		return winPathErr
	}
	if err := winNativeHost(); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil || (!strings.EqualFold(self, filepath.Join(BinDir, "loom.exe")) && !strings.EqualFold(self, filepath.Join(BinDir, "loom-updater.exe"))) {
		return errors.New("native Windows updates require the installed fixed executables")
	}
	if err := winPublicPaths(); err != nil {
		return err
	}
	if err := winTrusted(markerPath, windows.FILE_GENERIC_READ, false); err != nil {
		return err
	}
	raw, err := os.ReadFile(markerPath)
	if err != nil || string(raw) != winMarker {
		return errors.New("native Windows enrollment marker missing")
	}
	return nil
}
func enrollmentStatus() error {
	p := filepath.Join(BinDir, "updater-enrolled")
	if err := winTrusted(p, windows.FILE_GENERIC_READ, false); err != nil {
		return err
	}
	raw, err := os.ReadFile(p) // #nosec G304 -- fixed protected enrollment proof
	if err != nil || string(raw) != updates.TrustFingerprint() {
		return errors.New("matching Windows helper/trust-root enrollment required")
	}
	return nil
}
func winPublicPaths() error {
	for _, p := range []string{BinDir, filepath.Dir(ConfigDir)} {
		if err := winTrusted(p, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_EXECUTE, false); err != nil {
			return err
		}
	}
	for _, name := range []string{"loom.exe", "loom-updater.exe"} {
		if err := winTrusted(filepath.Join(BinDir, name), windows.FILE_GENERIC_READ|windows.FILE_GENERIC_EXECUTE, false); err != nil {
			return err
		}
	}
	return nil
}
func winPreflight() error {
	if !windowsCandidate {
		return errors.New("native Windows updater is an acceptance candidate, not production-enabled")
	}
	if winPathErr != nil {
		return winPathErr
	}
	if err := winNativeHost(); err != nil {
		return err
	}
	for _, path := range []string{BinDir, ConfigDir, DataDir, StateDir} {
		root, err := windows.UTF16PtrFromString(filepath.VolumeName(path) + `\`)
		if err != nil {
			return err
		}
		if windows.GetDriveType(root) != windows.DRIVE_FIXED {
			return errors.New("native Windows updater requires local fixed NTFS volumes")
		}
		var fs [32]uint16
		if err := windows.GetVolumeInformation(root, nil, 0, nil, nil, nil, &fs[0], uint32(len(fs))); err != nil {
			return err
		}
		if windows.UTF16ToString(fs[:]) != "NTFS" {
			return errors.New("native Windows automatic installation requires local NTFS")
		}
	}
	for _, name := range []string{"Loom", winHelperService} {
		s, err := winOpenService(name, windows.SERVICE_QUERY_CONFIG)
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			continue
		}
		if err != nil {
			return err
		}
		cfg, err := s.Config()
		_ = s.Close()
		if err != nil {
			return err
		}
		if err := winValidateService(name, cfg); err != nil {
			return errors.New("existing LocalService/custom installations cannot be silently migrated; retain manual updates")
		}
	}
	for path, rights := range map[string]uint32{BinDir: windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE, filepath.Dir(ConfigDir): windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE, ConfigDir: 0x1301bf, DataDir: 0x1301bf} {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := winTrusted(path, rights, false); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(StateDir); err == nil {
		if err := winTrusted(StateDir, 0, true); err != nil {
			return err
		}
		if _, err := os.Lstat(filepath.Join(StateDir, "journal.json")); !errors.Is(err, os.ErrNotExist) {
			return errors.New("recover pending Windows transaction before changing installed files")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func winCreateDir(path, sddl string, rights uint32, private bool) error {
	sa, err := winObjectSA(sddl)
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := windows.CreateDirectory(p, sa); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	return winTrusted(path, rights, private)
}
func winPrepare() error {
	if err := winPreflight(); err != nil {
		return err
	}
	for _, p := range []string{filepath.Dir(ConfigDir), BinDir} {
		if err := winCreateDir(p, winSDDL("FRFX", true), windows.FILE_GENERIC_READ|windows.FILE_GENERIC_EXECUTE, false); err != nil {
			return err
		}
	}
	for _, p := range []string{ConfigDir, DataDir, filepath.Join(filepath.Dir(ConfigDir), "logs")} {
		if err := winCreateDir(p, winSDDL("0x1301bf", true), 0x1301bf, false); err != nil {
			return err
		}
	}
	if err := winCreateDir(StateDir, winSDDL("", true), 0, true); err != nil {
		return err
	}
	config := filepath.Join(ConfigDir, "loom.yaml")
	if _, err := os.Lstat(config); errors.Is(err, os.ErrNotExist) {
		raw := fmt.Sprintf("config_dir: '%s'\ndata_dir: '%s'\nhttp:\n  addr: '127.0.0.1:1925'\nstorage:\n  engine: sqlite\n  sqlite:\n    path: '%s'\n", strings.ReplaceAll(ConfigDir, "'", "''"), strings.ReplaceAll(DataDir, "'", "''"), strings.ReplaceAll(DBPath, "'", "''"))
		if err := os.WriteFile(config, []byte(raw), 0600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return winFlushVolume(StateDir)
}

func winWriteProof(path, value string) error {
	if _, err := os.Lstat(path); err == nil {
		if err := winTrusted(path, windows.FILE_GENERIC_READ, false); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		return err
	}
	if err := winProtect(path, winSDDL("FR", false)); err != nil {
		return err
	}
	if err := syncFile(path); err != nil {
		return err
	}
	return winFlushVolume(path)
}
func winServiceSecurity(s *mgr.Service) error {
	text := "O:SYG:SYD:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x00020085;;;" + winServiceSID + ")"
	return winSetSD(s.Handle, text, windows.SE_SERVICE, false)
}
func winEnroll(ctx context.Context) error {
	if err := winPrepare(); err != nil {
		return err
	}
	if err := winPublicPaths(); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(StateDir, "journal.json")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("recover pending Windows transaction before enrollment")
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	if err := winWriteProof(tokenPath, hex.EncodeToString(random)); err != nil {
		return err
	}
	if err := winWriteProof(markerPath, winMarker); err != nil {
		return err
	}
	if err := winAtomicJSON(StateDir, "enrollment-pending.json", true); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(BinDir, "updater-enrolled")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	helper, err := m.OpenService(winHelperService)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		helper, err = m.CreateService(winHelperService, filepath.Join(BinDir, "loom-updater.exe"), mgr.Config{ServiceType: windows.SERVICE_WIN32_OWN_PROCESS, StartType: mgr.StartManual, ServiceStartName: "LocalSystem", DisplayName: "Loom privileged updater", Description: "Loom fixed-purpose native updater"}, "service")
	}
	if err != nil {
		return err
	}
	defer func() { _ = helper.Close() }()
	if err := winServiceSecurity(helper); err != nil {
		return err
	}
	main, err := m.OpenService("Loom")
	cfg := mgr.Config{ServiceType: windows.SERVICE_WIN32_OWN_PROCESS, StartType: mgr.StartAutomatic, DelayedAutoStart: true, ServiceStartName: winAccount, Dependencies: []string{winHelperService}, DisplayName: "Loom", Description: "Loom native installer - media automation", SidType: windows.SERVICE_SID_TYPE_UNRESTRICTED}
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		main, err = m.CreateService("Loom", filepath.Join(BinDir, "loom.exe"), cfg, "service", "--config", filepath.Join(ConfigDir, "loom.yaml"), "--log-file", filepath.Join(filepath.Dir(DataDir), "logs", "loom.log"))
	}
	if err != nil {
		return err
	}
	defer func() { _ = main.Close() }()
	if err := winServiceSecurity(main); err != nil {
		return err
	}
	// Explicit command strings avoid version-dependent command-line quoting.
	cfg.BinaryPathName = winMainCommand()
	if err := main.UpdateConfig(cfg); err != nil {
		return err
	}
	if err := winServicePrivileges(main.Handle, true); err != nil {
		return err
	}
	hcfg, err := helper.Config()
	if err != nil {
		return err
	}
	hcfg.BinaryPathName = winHelperCommand()
	hcfg.StartType = mgr.StartAutomatic
	if err := helper.UpdateConfig(hcfg); err != nil {
		return err
	}
	if err := helper.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 10 * time.Second}}, 86400); err != nil {
		return err
	}
	if err := helper.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return err
	}
	if err := helper.Start(); err != nil {
		return err
	}
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	for {
		if _, err := os.Stat(filepath.Join(StateDir, "enrollment-pending.json")); errors.Is(err, os.ErrNotExist) {
			return enrollmentStatus()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

type winHelperHandler struct{ ctx context.Context }

func (h *winHelperHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	statuses <- svc.Status{State: svc.StartPending, WaitHint: 300000}
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- winServe(ctx, func() {
			select {
			case ready <- struct{}{}:
			default:
			}
		})
	}()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	current := svc.Status{State: svc.StartPending, WaitHint: 300000}
	for {
		select {
		case <-ready:
			current = svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
			statuses <- current
		case err := <-done:
			if err != nil {
				return true, 1
			}
			return false, 0
		case req, ok := <-requests:
			if !ok {
				requests = nil
				cancel()
				current = svc.Status{State: svc.StopPending, WaitHint: 300000}
				statuses <- current
				continue
			}
			// SCM authenticates this one fixed maintenance control using
			// SERVICE_USER_DEFINED_CONTROL; Loom's SID is not granted it.
			if req.Cmd == svc.Stop || req.Cmd == svc.Shutdown || req.Cmd == 128 {
				cancel()
				current = svc.Status{State: svc.StopPending, WaitHint: 300000}
			}
			statuses <- current
		case <-tick.C:
			if current.State != svc.Running {
				current.CheckPoint++
				statuses <- current
			}
		}
	}
}

func winServe(ctx context.Context, announce func()) error {
	if uid, err := winUser(windows.GetCurrentProcessToken()); err != nil || uid != "S-1-5-18" {
		return errors.New("updater service must run as LocalSystem")
	}
	if err := winEnablePrivileges(); err != nil {
		return err
	}
	if err := winExposeIdentity(); err != nil {
		return err
	}
	var anchors []*os.File
	defer func() {
		for _, f := range anchors {
			_ = f.Close()
		}
	}()
	for _, path := range []string{BinDir, filepath.Dir(ConfigDir), ConfigDir, DataDir, StateDir} {
		f, err := winOpen(path, 0)
		if err != nil {
			return err
		}
		anchors = append(anchors, f)
	}
	lock, err := winLock()
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	gate, err := winNewGate()
	if err != nil {
		return err
	}
	defer gate.close()
	raw, err := os.ReadFile(tokenPath)
	if err != nil || len(raw) != 64 {
		return errors.New("invalid native token")
	}
	_, pending := os.Stat(filepath.Join(StateDir, "enrollment-pending.json"))
	control := &winControl{gate: gate, token: string(raw), enrolling: pending == nil, announce: announce}
	if err := control.validate(ctx); err != nil {
		return err
	}
	tx := winTransaction{service: control, publisher: &winPublisher{Checker: updates.NewChecker("dev")}}
	if control.enrolling {
		if _, err := os.Stat(filepath.Join(StateDir, "journal.json")); !errors.Is(err, os.ErrNotExist) {
			return errors.New("enrollment cannot bypass pending recovery")
		}
		if err := control.start(ctx); err != nil {
			return err
		}
		deadline, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		for {
			r, err := control.runtime(deadline)
			if err == nil && r.Native && r.Ready && r.Engine == "sqlite" && updates.ValidSource(r.Version, r.Ref, r.SHA, r.Channel) {
				if err := control.healthy(deadline, r.Identity); err != nil {
					return err
				}
				if err := winAtomicJSON(StateDir, "state.json", state{Installed: r.Identity}); err != nil {
					return err
				}
				if err := winWriteProof(filepath.Join(BinDir, "updater-enrolled"), updates.TrustFingerprint()); err != nil {
					return err
				}
				if err := os.Remove(filepath.Join(StateDir, "enrollment-pending.json")); err != nil {
					return err
				}
				if err := winFlushVolume(StateDir); err != nil {
					return err
				}
				control.enrolling = false
				break
			}
			select {
			case <-deadline.Done():
				return errors.New("native Windows enrollment requires ready SQLite/source identity")
			case <-time.After(time.Second):
			}
		}
	}
	for {
		cycle, cancel := context.WithTimeout(ctx, 15*time.Minute)
		if _, err := os.Stat(filepath.Join(StateDir, "journal.json")); err == nil {
			if err := tx.recover(cycle); err != nil {
				cancel()
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			cancel()
			return err
		} else if err := control.start(cycle); err != nil {
			cancel()
			return err
		}
		err := tx.run(cycle)
		cancel()
		if err != nil {
			if _, pending := os.Stat(filepath.Join(StateDir, "journal.json")); !errors.Is(pending, os.ErrNotExist) {
				return errors.New("native Windows recovery remains pending")
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Hour):
		}
	}
}

// Main accepts only fixed Windows installer/SCM operations; never caller paths.
func Main(ctx context.Context, args []string) error {
	if len(args) == 1 && args[0] == "snapshot" {
		return winSnapshotChild(ctx)
	}
	if len(args) != 1 {
		return errors.New("native Windows helper accepts only fixed preflight/prepare/enroll/service/maintenance operations")
	}
	if !windowsCandidate {
		return errors.New("native Windows updater awaits acceptance validation; production enrollment is disabled")
	}
	if args[0] == "service" {
		isService, err := svc.IsWindowsService()
		if err != nil || !isService {
			return errors.New("updater service must be launched by SCM")
		}
		return svc.Run(winHelperService, &winHelperHandler{ctx})
	}
	if !winAdmin() {
		return errors.New("native Windows updater enrollment/maintenance requires an elevated administrator")
	}
	switch args[0] {
	case "preflight":
		return winPreflight()
	case "prepare":
		if err := winEnablePrivileges(); err != nil {
			return err
		}
		return winPrepare()
	case "enroll":
		if err := winEnablePrivileges(); err != nil {
			return err
		}
		return winEnroll(ctx)
	case "maintenance":
		return winMaintenance(ctx)
	default:
		return errors.New("unknown fixed Windows updater operation")
	}
}

func winLock() (*os.File, error) {
	if err := winTrusted(StateDir, 0, true); err != nil {
		return nil, err
	}
	path, err := windows.UTF16PtrFromString(filepath.Join(StateDir, "lock"))
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, errors.New("another Windows updater owns recovery")
	}
	return os.NewFile(uintptr(h), filepath.Join(StateDir, "lock")), nil
}
func winMaintenance(ctx context.Context) error {
	if err := winPreflight(); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(StateDir, "journal.json")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("recover the pending transaction before maintenance")
	}
	main, err := winOpenService("Loom", windows.SERVICE_CHANGE_CONFIG|windows.SERVICE_QUERY_CONFIG)
	if err == nil {
		c, e := main.Config()
		if e == nil {
			c.StartType = mgr.StartDisabled
			e = main.UpdateConfig(c)
		}
		if e == nil {
			e = main.ResetRecoveryActions()
		}
		_ = main.Close()
		if e != nil {
			return e
		}
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	helper, err := winOpenService(winHelperService, windows.SERVICE_USER_DEFINED_CONTROL|windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG|windows.SERVICE_CHANGE_CONFIG)
	if err == nil {
		defer func() { _ = helper.Close() }()
		// A draining transaction may temporarily restart the old main for
		// rollback health. Prevent its SCM dependency/failure actions from
		// restarting the helper while the installer replaces either image.
		cfg, e := helper.Config()
		if e != nil {
			return e
		}
		cfg.StartType = mgr.StartDisabled
		if err := helper.UpdateConfig(cfg); err != nil {
			return err
		}
		if err := helper.ResetRecoveryActions(); err != nil {
			return err
		}
		s, e := helper.Query()
		if e != nil {
			return e
		}
		if s.State != svc.Stopped && s.State != svc.StopPending {
			if _, err := helper.Control(svc.Cmd(128)); err != nil {
				return err
			}
		}
		for {
			s, e = helper.Query()
			if e != nil {
				return e
			}
			if s.State == svc.Stopped {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	lock, err := winLock()
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if _, err := os.Stat(filepath.Join(StateDir, "journal.json")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("native Windows recovery remains pending; binaries retained")
	}
	g, err := winNewGate()
	if err != nil {
		return err
	}
	defer g.close()
	return (&winControl{gate: g}).stop(ctx)
}

func winExposeIdentity() error {
	// The service may query identity/liveness, never duplicate the SYSTEM token
	// or modify the privileged process. This authenticates the fixed startup IPC.
	process := "O:SYG:SYD:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x00101000;;;" + winServiceSID + ")"
	if err := winSetSD(windows.CurrentProcess(), process, windows.SE_KERNEL_OBJECT, false); err != nil {
		return err
	}
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.WRITE_DAC|windows.WRITE_OWNER|windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer func() { _ = token.Close() }()
	return winSetSD(windows.Handle(token), "O:SYG:SYD:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x8;;;"+winServiceSID+")", windows.SE_KERNEL_OBJECT, false)
}

type winPublisher struct{ *updates.Checker }

func (p *winPublisher) Discover(ctx context.Context, channel string) (updates.Candidate, error) {
	var next map[string]time.Time
	if err := readJSON(filepath.Join(StateDir, "next-check.json"), &next); err != nil && !errors.Is(err, os.ErrNotExist) {
		return updates.Candidate{}, err
	}
	if time.Now().Before(next[channel]) {
		return updates.Candidate{}, errors.New("native metadata check is throttled")
	}
	c, err := p.Checker.Discover(ctx, channel)
	if next == nil {
		next = map[string]time.Time{}
	}
	next[channel] = p.Checker.Check(ctx, channel).NextCheck
	if e := winAtomicJSON(StateDir, "next-check.json", next); e != nil {
		return updates.Candidate{}, e
	}
	return c, err
}
