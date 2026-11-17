//go:build windows

package updater

import (
	"archive/zip"
	"crypto/sha1" // #nosec G505 -- Windows service SID derivation, not signature verification
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

func TestWindowsFixedAccountAndPaths(t *testing.T) {
	words := utf16.Encode([]rune("LOOM"))
	raw := make([]byte, len(words)*2)
	for i, w := range words {
		binary.LittleEndian.PutUint16(raw[2*i:], w)
	}
	hash := sha1.Sum(raw) // #nosec G401 -- documented NT SERVICE SID algorithm
	sid := "S-1-5-80"
	for i := 0; i < 5; i++ {
		sid += fmt.Sprintf("-%d", binary.LittleEndian.Uint32(hash[i*4:]))
	}
	if sid != winServiceSID {
		t.Fatal("virtual-account SID drift")
	}
	for _, bad := range []string{`\\server\share\loom.exe`, `C:relative.exe`, `C:\foo\..\loom.exe`, `C:\foo\loom.exe:stream`, `C:\foo\file.`, `C:\foo\file `, `C:\foo/NUL`, `C:\foo\NUL`, `C:\foo\COM1.txt`, `C:\foo\LPT9.log`, `C:\foo\COM¹.txt`} {
		if winSafeAbsolute(bad) {
			t.Errorf("accepted unsafe path %q", bad)
		}
	}
	if !winSafeAbsolute(`C:\Program Files\Loom\loom.exe`) {
		t.Fatal("fixed path rejected")
	}
	for _, bad := range []string{`..\outside`, `C:\absolute`, `file:ads`, `foo/bar`, `CON`, `nested\COM1.txt`} {
		if winSafeRelative(bad) {
			t.Errorf("accepted journal escape %q", bad)
		}
	}
}

func TestWindowsNTFSStreamInfoValidation(t *testing.T) {
	streamInfo := func(next uint32, name string) []byte {
		encoded := utf16.Encode([]rune(name))
		info := make([]byte, 24+len(encoded)*2)
		binary.LittleEndian.PutUint32(info[:4], next)
		binary.LittleEndian.PutUint32(info[4:8], uint32(len(encoded)*2))
		for i, unit := range encoded {
			binary.LittleEndian.PutUint16(info[24+i*2:], unit)
		}
		return info
	}
	multiple := make([]byte, 40+len(streamInfo(0, "::$DATA:private")))
	copy(multiple, streamInfo(40, "::$DATA"))
	copy(multiple[40:], streamInfo(0, "::$DATA:private"))

	for _, tc := range []struct {
		name string
		dir  bool
		info []byte
		ok   bool
	}{
		{"ordinary-file", false, streamInfo(0, "::$DATA"), true},
		{"ordinary-directory", true, streamInfo(0, ":$I30:$INDEX_ALLOCATION"), true},
		{"file-alternate-stream", false, streamInfo(0, "::$DATA:private"), false},
		{"directory-alternate-stream", true, streamInfo(0, ":private:$DATA"), false},
		{"unexpected-directory-stream", true, streamInfo(0, "::$DATA"), false},
		{"multiple-streams", false, multiple, false},
		{"truncated-entry", true, []byte{1, 2, 3}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := winValidateStreamInfo(tc.info, tc.dir)
			if (err == nil) != tc.ok {
				t.Fatalf("validation error=%v want accepted=%v", err, tc.ok)
			}
		})
	}
}

func TestWindowsNTFSStreamInfoBufferAlignment(t *testing.T) {
	for _, tc := range []struct {
		name            string
		size, alignment int
	}{
		{"EA information", 4, 4},
		{"stream information", 8192, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buffer := winAlignedNativeBuffer(tc.size, tc.alignment)
			if len(buffer) != tc.size {
				t.Fatalf("buffer length=%d want=%d", len(buffer), tc.size)
			}
			if address := uintptr(unsafe.Pointer(&buffer[0])); address%uintptr(tc.alignment) != 0 { // #nosec G103 -- inspect alignment in this regression test
				t.Fatalf("buffer address %#x is not %d-byte aligned", address, tc.alignment)
			}
		})
	}
}

func TestWindowsNTQueryFailureIncludesCallAndStatus(t *testing.T) {
	status := windows.NTStatus(0x80000002) // STATUS_DATATYPE_MISALIGNMENT
	err := winNTQueryError("NtQueryInformationFile(FileEaInformation)", status)
	if !strings.Contains(err.Error(), "NtQueryInformationFile(FileEaInformation), NTSTATUS 0x80000002") {
		t.Fatalf("native query diagnostic=%q", err)
	}
	if !errors.Is(err, status) {
		t.Fatal("native query diagnostic does not preserve the NTSTATUS")
	}
}

func TestWindowsTrustRoles(t *testing.T) {
	rx := uint32(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE)
	for _, tc := range []struct {
		name, sddl  string
		rights      uint32
		private, ok bool
	}{
		{"binary", winSDDL("FRFX", false), rx, false, true},
		{"private", winSDDL("", true), 0, true, true},
		{"service-private", winSDDL("FR", false), rx, true, false},
		{"service-writable-binary", winSDDL("FA", false), rx, false, false},
		{"service-owned-binary", strings.Replace(winSDDL("FRFX", false), "O:SY", "O:"+winServiceSID, 1), rx, false, false},
		{"unknown-trustee", winSDDL("FR", false) + "(A;;FR;;;BU)", rx, false, false},
		{"null-dacl", "O:SYG:SYD:NO_ACCESS_CONTROL", rx, false, false},
		{"gate", winKernelSD("event"), windows.SYNCHRONIZE | windows.READ_CONTROL, false, true},
		{"job", winKernelSD("job"), winJobAssign | winJobQuery | windows.READ_CONTROL, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(tc.sddl)
			if err != nil {
				t.Fatal(err)
			}
			if err := winCheckSD(sd, tc.rights, tc.private); (err == nil) != tc.ok {
				t.Fatalf("trust result %v, want accepted=%v", err, tc.ok)
			}
		})
	}
}

func TestWindowsCanonicalSCM(t *testing.T) {
	good := mgr.Config{ServiceType: windows.SERVICE_WIN32_OWN_PROCESS, BinaryPathName: winMainCommand(), ServiceStartName: winAccount, SidType: windows.SERVICE_SID_TYPE_UNRESTRICTED, Dependencies: []string{winHelperService}}
	if err := winValidateService("Loom", good); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*mgr.Config){
		func(c *mgr.Config) { c.ServiceStartName = `NT AUTHORITY\LocalService` },
		func(c *mgr.Config) { c.BinaryPathName += ` --arbitrary-path C:\outside` },
		func(c *mgr.Config) { c.Dependencies = nil },
		func(c *mgr.Config) { c.ServiceType = windows.SERVICE_WIN32_SHARE_PROCESS },
		func(c *mgr.Config) { c.SidType = windows.SERVICE_SID_TYPE_NONE },
	} {
		bad := good
		mutate(&bad)
		if err := winValidateService("Loom", bad); err == nil {
			t.Fatal("custom SCM configuration accepted")
		}
	}
}

func TestWindowsControlQuiescent(t *testing.T) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(job) }()
	if err := (&winControl{gate: &winGate{job: job}}).quiescent(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsArchiveExactExecutableOnly(t *testing.T) {
	for _, names := range [][]string{{"loom.exe"}, {"loom.exe", "../outside", "loom-updater.exe"}, {"loom.exe", "loom.exe"}, {"LOOM.EXE"}, {"nested/loom.exe"}, {"loom.exe", "LOOM.EXE"}} {
		t.Run(strings.Join(names, "_"), func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "asset.zip")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			z := zip.NewWriter(file)
			for _, name := range names {
				w, err := z.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.Write([]byte("binary")); err != nil {
					t.Fatal(err)
				}
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(dir, "new")
			err = winExtractZIP(archive, dest)
			want := reflect.DeepEqual(names, []string{"loom.exe"}) || reflect.DeepEqual(names, []string{"loom.exe", "../outside", "loom-updater.exe"})
			if (err == nil) != want {
				t.Fatalf("archive result %v want success=%v", err, want)
			}
			if _, err := os.Stat(filepath.Join(dir, "outside")); !os.IsNotExist(err) {
				t.Fatal("extracted a caller path")
			}
		})
	}
}

func TestWindowsDefaultEnrollmentFailsClosed(t *testing.T) {
	if windowsCandidate {
		t.Skip("candidate has separate privileged native acceptance requirements")
	}
	if supportedNative {
		t.Fatal("unvalidated Windows capability enabled")
	}
	for _, args := range [][]string{nil, {"enroll"}, {"service"}, {"prepare"}, {"recover"}, {"snapshot"}, {"apply", "C:\\other.exe"}} {
		if err := Main(t.Context(), args); err == nil {
			t.Fatalf("unsupported operation allowed: %v", args)
		}
	}
	if ok, reason := Capability(Binding{Engine: "postgres"}); ok || !strings.Contains(reason, "PostgreSQL") {
		t.Fatal("missing PostgreSQL fallback")
	}
	if ok, reason := Capability(Binding{Engine: "sqlite"}); ok || !strings.Contains(reason, "manual-only") {
		t.Fatal("Windows production activation bypass")
	}
}
