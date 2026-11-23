//go:build windows

package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	winJobAssign = 0x0001
	winJobQuery  = 0x0004
)

type winGate struct{ event, job windows.Handle }

func winKernelSD(kind string) string {
	rights := "0x00120000" // SYNCHRONIZE | READ_CONTROL, not EVENT_MODIFY_STATE
	if kind == "job" {
		rights = "0x00020005"
	} // ASSIGN_PROCESS | QUERY | READ_CONTROL
	return "O:SYG:SYD:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;" + rights + ";;;" + winServiceSID + ")"
}
func winObjectSA(sddl string) (*windows.SecurityAttributes, error) {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}, nil
}
func winKernelSA(kind string) (*windows.SecurityAttributes, error) {
	sddl := winKernelSD(kind)
	if user, err := winUser(windows.GetCurrentProcessToken()); err != nil {
		return nil, err
	} else if user != "S-1-5-18" {
		// Elevated maintenance may recreate an otherwise destroyed object.
		// BA is a trusted owner and avoids assigning SYSTEM as a foreign
		// kernel-object owner from an administrator's token.
		sddl = strings.Replace(sddl, "O:SY", "O:BA", 1)
	}
	return winObjectSA(sddl)
}
func winCheckKernel(h windows.Handle, rights uint32) error {
	sd, err := windows.GetSecurityInfo(h, windows.SE_KERNEL_OBJECT, winReadSD)
	if err != nil {
		return err
	}
	return winCheckSD(sd, rights, false)
}

//nolint:gosec // G103: fixed Windows job information ABI, live stack buffers passed synchronously.
func winNewGate() (*winGate, error) {
	sa, err := winKernelSA("event")
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(winGateName)
	if err != nil {
		return nil, err
	}
	ev, err := windows.CreateEvent(sa, 1, 0, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	g := &winGate{event: ev}
	if err := winCheckKernel(ev, windows.SYNCHRONIZE|windows.READ_CONTROL); err != nil {
		g.close()
		return nil, err
	}
	if err := windows.ResetEvent(ev); err != nil {
		g.close()
		return nil, err
	}
	sa, err = winKernelSA("job")
	if err != nil {
		g.close()
		return nil, err
	}
	name, err = windows.UTF16PtrFromString(winJobName)
	if err != nil {
		g.close()
		return nil, err
	}
	g.job, err = windows.CreateJobObject(sa, name)
	if err != nil {
		g.close()
		return nil, err
	}
	if err := winCheckKernel(g.job, winJobAssign|winJobQuery|windows.READ_CONTROL); err != nil {
		g.close()
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(g.job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		g.close()
		return nil, err
	}
	return g, nil
}
func (g *winGate) close() {
	if g.event != 0 {
		_ = windows.ResetEvent(g.event)
		_ = windows.CloseHandle(g.event)
	}
	if g.job != 0 {
		_ = windows.CloseHandle(g.job)
	}
}

//nolint:gosec // G103: fixed JOBOBJECT_BASIC_ACCOUNTING_INFORMATION ABI.
func (g *winGate) empty() error {
	var accounting struct {
		TotalUser, TotalKernel, ThisUser, ThisKernel            int64
		PageFaults, TotalProcesses, ActiveProcesses, Terminated uint32
	}
	if err := windows.QueryInformationJobObject(g.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
		return err
	}
	if accounting.ActiveProcesses != 0 {
		return errors.New("native Loom process job is not empty")
	}
	return nil
}

func winOpenService(name string, rights uint32) (*mgr.Service, error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseServiceHandle(scm) }()
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := windows.OpenService(scm, n, rights)
	if err != nil {
		return nil, err
	}
	return &mgr.Service{Name: name, Handle: h}, nil
}

func winProcess(pid uint32, path, sid string) (windows.Handle, windows.Token, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return 0, 0, err
	}
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		_ = windows.CloseHandle(h)
		return 0, 0, err
	}
	uid, err := winUser(token)
	if err == nil && sid == winServiceSID {
		err = winUnprivilegedAccount(token)
	}
	var image [32768]uint16
	n := uint32(len(image))
	if err == nil {
		err = windows.QueryFullProcessImageName(h, 0, &image[0], &n)
	}
	if err != nil || uid != sid || !strings.EqualFold(windows.UTF16ToString(image[:n]), path) {
		_ = token.Close()
		_ = windows.CloseHandle(h)
		return 0, 0, errors.New("SCM process image/account does not match enrolled Loom")
	}
	return h, token, nil
}

// StartupGuard authenticates the fixed SCM helper before any service-owned I/O.
func StartupGuard(ctx context.Context) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.EqualFold(self, filepath.Join(BinDir, "loom.exe")) {
		return nil
	}
	if _, err := os.Lstat(markerPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := nativeInstalled(); err != nil {
		return err
	}
	uid, err := winUser(windows.GetCurrentProcessToken())
	if err != nil || uid != winServiceSID || winUnprivilegedAccount(windows.GetCurrentProcessToken()) != nil {
		return errors.New("enrolled Loom requires its dedicated virtual service account")
	}
	own, err := winOpenService("Loom", windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return err
	}
	defer func() { _ = own.Close() }()
	s, err := own.Query()
	if err != nil || int64(s.ProcessId) != int64(os.Getpid()) {
		return errors.New("only the installed SCM Loom process may pass startup")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		if err := winAwaitGate(); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("native Windows updater recovery has not authorized database startup")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func winUnprivilegedAccount(token windows.Token) error {
	groups, err := token.GetTokenGroups()
	if err != nil {
		return err
	}
	for _, g := range groups.AllGroups() {
		if g.Sid.String() == "S-1-5-32-544" && g.Attributes&windows.SE_GROUP_ENABLED != 0 {
			return errors.New("native Loom virtual account must not be an administrator")
		}
	}
	return nil
}

//nolint:gosec // G103: documented OpenJobObjectW/IsProcessInJob ABI; fixed names and bounded native outputs.
func winAwaitGate() error {
	helper, err := winOpenService(winHelperService, windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return err
	}
	defer func() { _ = helper.Close() }()
	status, err := helper.Query()
	if err != nil || status.State != svc.Running || status.ProcessId == 0 {
		return errors.New("privileged updater is not running")
	}
	proc, token, err := winProcess(status.ProcessId, filepath.Join(BinDir, "loom-updater.exe"), "S-1-5-18")
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(proc) }()
	defer func() { _ = token.Close() }()
	name, err := windows.UTF16PtrFromString(winGateName)
	if err != nil {
		return err
	}
	ev, err := windows.OpenEvent(windows.SYNCHRONIZE|windows.READ_CONTROL, false, name)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(ev) }()
	if err := winCheckKernel(ev, windows.SYNCHRONIZE|windows.READ_CONTROL); err != nil {
		return err
	}
	// OpenJobObject has no x/sys wrapper. All names/rights are protocol constants.
	p := windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenJobObjectW")
	name, err = windows.UTF16PtrFromString(winJobName)
	if err != nil {
		return err
	}
	raw, _, callErr := p.Call(winJobAssign|winJobQuery|windows.READ_CONTROL, 0, uintptr(unsafe.Pointer(name)))
	if raw == 0 {
		return callErr
	}
	job := windows.Handle(raw)
	defer func() { _ = windows.CloseHandle(job) }()
	if err := winCheckKernel(job, winJobAssign|winJobQuery|windows.READ_CONTROL); err != nil {
		return err
	}
	var inJob uint32
	check := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")
	ok, _, e := check.Call(uintptr(windows.CurrentProcess()), uintptr(job), uintptr(unsafe.Pointer(&inJob)))
	if ok == 0 {
		return e
	}
	if inJob == 0 {
		if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
			return err
		}
	}
	// A stale signaled event is insufficient: authenticate a live SCM helper
	// process as well. A helper restart resets the event before reporting Running.
	if state, err := windows.WaitForSingleObject(proc, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return errors.New("updater process exited")
	}
	if state, err := windows.WaitForSingleObject(ev, 0); err != nil || state != windows.WAIT_OBJECT_0 {
		return errors.New("startup gate closed")
	}
	return nil
}
