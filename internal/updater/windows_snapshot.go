//go:build windows

package updater

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func winSnapshotChild(ctx context.Context) error {
	uid, err := winUser(windows.GetCurrentProcessToken())
	if err != nil || uid != winServiceSID || winSnapshotPrivileges(windows.GetCurrentProcessToken()) != nil {
		return errors.New("snapshot requires the restricted Loom virtual-account token")
	}
	dir, err := os.MkdirTemp(DataDir, ".loom-snapshot-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	return vacuumToStream(ctx, filepath.ToSlash(DBPath), filepath.Join(dir, "snapshot.db"), os.Stdout)
}

//nolint:gosec // G103: fixed CreateRestrictedToken and job information ABIs, no caller pointers.
func (s *winControl) snapshotDB(ctx context.Context, dest string) error {
	if s.snapshotToken == 0 {
		return errors.New("no authenticated pre-stop service token")
	}
	var restricted windows.Token
	create := windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken")
	ok, _, e := create.Call(uintptr(s.snapshotToken), 1, 0, 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&restricted))) // DISABLE_MAX_PRIVILEGE
	if ok == 0 {
		return e
	}
	defer func() { _ = restricted.Close() }()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) // #nosec G304 -- fixed private SQLite backup destination
	if err != nil {
		return err
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		_ = out.Close()
		return err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(BinDir, "loom-updater.exe"), "snapshot") // #nosec G204 -- installed protected helper and one literal operation, no caller arguments
	cmd.Env = []string{"SystemRoot=" + filepath.Dir(system), "PATH=" + system, "TEMP=" + DataDir, "TMP=" + DataDir}
	cmd.Dir = DataDir
	cmd.WaitDelay = 5 * time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Token: syscall.Token(restricted), CreationFlags: windows.CREATE_SUSPENDED}
	cmd.Stdout = &winSnapshotWriter{out: out, remaining: 64 << 30}
	if err := cmd.Start(); err != nil {
		_ = out.Close()
		return err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err == nil {
		limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	}
	if job != 0 {
		defer func() { _ = windows.CloseHandle(job) }()
	}
	if err == nil {
		var proc windows.Handle
		proc, err = windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid)) // #nosec G115 -- Go's PID is a Windows DWORD
		if err == nil {
			err = windows.AssignProcessToJobObject(job, proc)
			_ = windows.CloseHandle(proc)
		}
	}
	if err == nil {
		err = winResumeSnapshot(uint32(cmd.Process.Pid)) // #nosec G115 -- Go's PID is a Windows DWORD
	}
	if err != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if job != 0 {
		_ = windows.TerminateJobObject(job, 1)
	}
	return errors.Join(err, waitErr, out.Sync(), out.Close())
}

//nolint:gosec // G103: GetTokenInformation buffer length/count validated before native struct access.
func winSnapshotPrivileges(token windows.Token) error {
	var needed uint32
	_ = windows.GetTokenInformation(token, windows.TokenPrivileges, nil, 0, &needed)
	if needed < uint32(unsafe.Sizeof(windows.Tokenprivileges{})) || needed > 64<<10 {
		return errors.New("invalid snapshot token privileges")
	}
	buf := make([]byte, needed)
	if err := windows.GetTokenInformation(token, windows.TokenPrivileges, &buf[0], needed, &needed); err != nil {
		return err
	}
	var traverse windows.LUID
	name, err := windows.UTF16PtrFromString("SeChangeNotifyPrivilege")
	if err != nil {
		return err
	}
	if err := windows.LookupPrivilegeValue(nil, name, &traverse); err != nil {
		return err
	}
	privs := (*windows.Tokenprivileges)(unsafe.Pointer(&buf[0]))
	if uint64(privs.PrivilegeCount)*uint64(unsafe.Sizeof(windows.LUIDAndAttributes{}))+4 > uint64(len(buf)) {
		return errors.New("invalid token privilege buffer")
	}
	for _, p := range privs.AllPrivileges() {
		if p.Luid != traverse {
			return errors.New("snapshot token retains privileges")
		}
	}
	return nil
}

func winResumeSnapshot(pid uint32) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snap, &entry); err == nil; err = windows.Thread32Next(snap, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		defer func() { _ = windows.CloseHandle(thread) }()
		_, err = windows.ResumeThread(thread)
		return err
	}
	return errors.New("suspended snapshot process has no primary thread")
}

type winSnapshotWriter struct {
	out       *os.File
	remaining int64
}

func (w *winSnapshotWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errors.New("snapshot exceeds 64 GiB")
	}
	n, err := w.out.Write(p)
	w.remaining -= int64(n)
	return n, err
}
