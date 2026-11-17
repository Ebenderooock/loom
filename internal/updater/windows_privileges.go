//go:build windows

package updater

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Virtual accounts otherwise inherit service privileges such as impersonation.
// Neither the main service nor its descendants need those privileges.
//
//nolint:gosec // G103: fixed SERVICE_REQUIRED_PRIVILEGES_INFO ABI, returned MULTI_SZ pointer bounded against its owning buffer.
func winServicePrivileges(service windows.Handle, set bool) error {
	text, err := windows.UTF16FromString("SeChangeNotifyPrivilege")
	if err != nil {
		return err
	}
	text = append(text, 0)
	multi := text
	info := struct{ Privileges *uint16 }{&multi[0]}
	if set {
		return windows.ChangeServiceConfig2(service, windows.SERVICE_CONFIG_REQUIRED_PRIVILEGES_INFO, (*byte)(unsafe.Pointer(&info)))
	}
	var needed uint32
	_ = windows.QueryServiceConfig2(service, windows.SERVICE_CONFIG_REQUIRED_PRIVILEGES_INFO, nil, 0, &needed)
	if needed < uint32(unsafe.Sizeof(info)) || needed > 64<<10 {
		return errors.New("invalid SCM privilege list")
	}
	buf := make([]byte, needed)
	if err := windows.QueryServiceConfig2(service, windows.SERVICE_CONFIG_REQUIRED_PRIVILEGES_INFO, &buf[0], needed, &needed); err != nil {
		return err
	}
	ptr := *(*uintptr)(unsafe.Pointer(&buf[0]))
	start := uintptr(unsafe.Pointer(&buf[0]))
	if ptr < start || ptr-start+uintptr(len(multi)*2) > uintptr(len(buf)) {
		return errors.New("SCM privilege pointer outside result")
	}
	offset := int(ptr - start)
	for i, want := range multi {
		if uint16(buf[offset+2*i])|uint16(buf[offset+2*i+1])<<8 != want {
			return errors.New("native Loom SCM privileges exceed traverse-only")
		}
	}
	return nil
}
