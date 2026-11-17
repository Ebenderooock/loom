//go:build windows

package updater

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type winControl struct {
	gate          *winGate
	token         string
	snapshotToken windows.Token
	enrolling     bool
	announce      func()
}

func winMainCommand() string {
	return `"` + filepath.Join(BinDir, "loom.exe") + `" service --config "` + filepath.Join(ConfigDir, "loom.yaml") + `" --log-file "` + filepath.Join(filepath.Dir(DataDir), "logs", "loom.log") + `"`
}
func winHelperCommand() string { return `"` + filepath.Join(BinDir, "loom-updater.exe") + `" service` }

func (s *winControl) validate(context.Context) error {
	if err := nativeInstalled(); err != nil {
		return err
	}
	if !s.enrolling {
		if err := enrollmentStatus(); err != nil {
			return err
		}
	}
	for _, p := range []string{ConfigDir, DataDir} {
		if err := winTrusted(p, 0x1301bf, false); err != nil {
			return err
		}
	}
	if err := winTrusted(StateDir, 0, true); err != nil {
		return err
	}
	for _, name := range []string{"Loom", winHelperService} {
		service, err := winOpenService(name, windows.SERVICE_QUERY_CONFIG|windows.READ_CONTROL)
		if err != nil {
			return err
		}
		cfg, err := service.Config()
		if err == nil {
			err = winValidateService(name, cfg)
		}
		if err == nil && name == "Loom" {
			err = winServicePrivileges(service.Handle, false)
		}
		sd, sdErr := windows.GetSecurityInfo(service.Handle, windows.SE_SERVICE, winReadSD)
		if err == nil {
			err = sdErr
		}
		if err == nil {
			err = winCheckSD(sd, windows.READ_CONTROL|windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS|windows.SERVICE_INTERROGATE, false)
		}
		_ = service.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func winValidateService(name string, c mgr.Config) error {
	if c.ServiceType != windows.SERVICE_WIN32_OWN_PROCESS {
		return errors.New("shared/custom SCM services cannot auto-update")
	}
	if name == "Loom" {
		if c.BinaryPathName != winMainCommand() || !strings.EqualFold(c.ServiceStartName, winAccount) ||
			c.SidType != windows.SERVICE_SID_TYPE_UNRESTRICTED || !slices.Equal(c.Dependencies, []string{winHelperService}) {
			return errors.New("SCM Loom configuration is not the fixed virtual-account installation")
		}
	} else if c.BinaryPathName != winHelperCommand() || !strings.EqualFold(c.ServiceStartName, "LocalSystem") || len(c.Dependencies) != 0 {
		return errors.New("SCM updater configuration changed")
	}
	return nil
}

func (s *winControl) runtime(ctx context.Context) (Runtime, error) {
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
		return r, errors.New("native runtime unavailable")
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&r)
	return r, err
}
func (s *winControl) captureToken() error {
	main, err := winOpenService("Loom", windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return err
	}
	defer func() { _ = main.Close() }()
	status, err := main.Query()
	if err != nil || status.ProcessId == 0 {
		return errors.New("no Loom process for restricted snapshot token")
	}
	proc, token, err := winProcess(status.ProcessId, filepath.Join(BinDir, "loom.exe"), winServiceSID)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(proc) }()
	defer func() { _ = token.Close() }()
	var duplicate windows.Token
	if err := windows.OpenProcessToken(proc, windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE, &duplicate); err != nil {
		return err
	}
	defer func() { _ = duplicate.Close() }()
	var copy windows.Token
	if err := windows.DuplicateTokenEx(duplicate, windows.TOKEN_ALL_ACCESS, nil, windows.SecurityImpersonation, windows.TokenPrimary, &copy); err != nil {
		return err
	}
	if s.snapshotToken != 0 {
		_ = s.snapshotToken.Close()
	}
	s.snapshotToken = copy
	return nil
}

func (s *winControl) releaseToken() {
	if s.snapshotToken != 0 {
		_ = s.snapshotToken.Close()
		s.snapshotToken = 0
	}
}

func (s *winControl) stop(ctx context.Context) error {
	if err := windows.ResetEvent(s.gate.event); err != nil {
		return err
	}
	main, err := winOpenService("Loom", windows.SERVICE_ALL_ACCESS)
	if err != nil {
		return err
	}
	defer func() { _ = main.Close() }()
	cfg, err := main.Config()
	if err != nil {
		return err
	}
	if err := winValidateService("Loom", cfg); err != nil {
		return err
	}
	// Disable queued SCM failure restarts during quiescence. The startup gate
	// also covers boot/crash restart; start always restores canonical settings.
	cfg.StartType = mgr.StartDisabled
	if err := main.UpdateConfig(cfg); err != nil {
		return err
	}
	if err := main.ResetRecoveryActions(); err != nil {
		return err
	}
	status, err := main.Query()
	if err != nil {
		return err
	}
	if status.State != svc.Stopped && status.State != svc.StopPending {
		if _, err := main.Control(svc.Stop); err != nil {
			return err
		}
	}
	deadline := time.NewTimer(120 * time.Second)
	defer deadline.Stop()
	for {
		status, err = main.Query()
		if err != nil {
			return err
		}
		if status.State == svc.Stopped {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("SCM stop timed out")
		case <-time.After(200 * time.Millisecond):
		}
	}
	if err := windows.TerminateJobObject(s.gate.job, 1); err != nil {
		return err
	}
	for {
		if err := s.quiescent(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("service account/job did not quiesce")
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func (s *winControl) quiescent(context.Context) error {
	if err := s.gate.empty(); err != nil {
		return err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == 0 || entry.ProcessID == 4 {
			continue
		}
		h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if e != nil {
			if errors.Is(e, windows.ERROR_INVALID_PARAMETER) || errors.Is(e, windows.ERROR_ACCESS_DENIED) {
				// Exited or protected system processes cannot be inspected. Loom
				// descendants are separately contained by the non-breakaway job.
				continue
			}
			return errors.New("cannot establish Windows process quiescence")
		}
		var token windows.Token
		e = windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token)
		_ = windows.CloseHandle(h)
		if e != nil {
			if errors.Is(e, windows.ERROR_ACCESS_DENIED) {
				continue
			}
			return errors.New("cannot inspect Windows process account")
		}
		uid, e := winUser(token)
		_ = token.Close()
		if e != nil {
			return e
		}
		if uid == winServiceSID {
			return errors.New("a Loom virtual-account process escaped or remains active")
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return err
	}
	return nil
}
func (s *winControl) start(context.Context) error {
	main, err := winOpenService("Loom", windows.SERVICE_ALL_ACCESS)
	if err != nil {
		return err
	}
	defer func() { _ = main.Close() }()
	cfg, err := main.Config()
	if err != nil {
		return err
	}
	if err := winValidateService("Loom", cfg); err != nil {
		return err
	}
	cfg.StartType = mgr.StartAutomatic
	cfg.DelayedAutoStart = true
	if err := main.UpdateConfig(cfg); err != nil {
		return err
	}
	if err := main.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 10 * time.Second}}, 86400); err != nil {
		return err
	}
	if err := main.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return err
	}
	if s.announce != nil {
		s.announce()
	}
	if err := windows.SetEvent(s.gate.event); err != nil {
		return err
	}
	status, err := main.Query()
	if err != nil {
		return err
	}
	if status.State == svc.Running || status.State == svc.StartPending {
		return nil
	}
	if err := main.Start(); err != nil {
		_ = windows.ResetEvent(s.gate.event)
		return err
	}
	return nil
}
func (s *winControl) healthy(ctx context.Context, want Identity) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for {
		r, err := s.runtime(ctx)
		if err == nil && r.Native && r.Ready && r.Engine == "sqlite" && sameIdentity(r.Identity, want) {
			main, err := winOpenService("Loom", windows.SERVICE_QUERY_STATUS)
			if err == nil {
				status, e := main.Query()
				_ = main.Close()
				if e == nil && status.State == svc.Running {
					h, t, e := winProcess(status.ProcessId, filepath.Join(BinDir, "loom.exe"), winServiceSID)
					if e == nil {
						_ = windows.CloseHandle(h)
						_ = t.Close()
						return nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("SCM candidate failed authenticated identity/readiness")
		case <-time.After(time.Second):
		}
	}
}
