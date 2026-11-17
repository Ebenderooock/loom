//go:build windows

package updater

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const winReadSD = windows.OWNER_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION

func winUser(token windows.Token) (string, error) {
	u, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}
func winAdmin() bool {
	u, err := winUser(windows.GetCurrentProcessToken())
	return err == nil && (u == "S-1-5-18" || windows.GetCurrentProcessToken().IsElevated())
}

func winEnablePrivileges() error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer func() { _ = token.Close() }()
	names := []string{"SeRestorePrivilege", "SeBackupPrivilege", "SeSecurityPrivilege", "SeManageVolumePrivilege"}
	if user, err := winUser(token); err == nil && user == "S-1-5-18" {
		names = append(names, "SeAssignPrimaryTokenPrivilege", "SeIncreaseQuotaPrivilege")
	}
	for _, name := range names {
		var id windows.LUID
		p, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return err
		}
		if err := windows.LookupPrivilegeValue(nil, p, &id); err != nil {
			return err
		}
		state := windows.Tokenprivileges{PrivilegeCount: 1, Privileges: [1]windows.LUIDAndAttributes{{Luid: id, Attributes: windows.SE_PRIVILEGE_ENABLED}}}
		if err := windows.AdjustTokenPrivileges(token, false, &state, 0, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

// No null DACL, deny/callback/object ACEs or unknown trustees. Service grants
// cannot include WRITE_DAC/WRITE_OWNER or access beyond this object's role.
func winCheckSD(sd *windows.SECURITY_DESCRIPTOR, serviceRights uint32, private bool) error {
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner.String() != "S-1-5-18" && owner.String() != "S-1-5-32-544" &&
		(private || serviceRights&windows.FILE_WRITE_DATA == 0 || owner.String() != winServiceSID) {
		return errors.New("untrusted Windows object owner")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount == 0 {
		return errors.New("missing restrictive Windows DACL")
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("custom Windows ACEs require manual updates")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)) // #nosec G103 -- GetAce returns the documented native SID-at-offset layout
		switch sid.String() {
		case "S-1-5-18", "S-1-5-32-544":
		case winServiceSID:
			if private || uint32(ace.Mask)&^serviceRights != 0 {
				return errors.New("service has excessive Windows object rights")
			}
		default:
			return errors.New("custom Windows DACL requires manual updates")
		}
	}
	return nil
}

func winSDDL(serviceRights string, dir bool) string {
	flags := ""
	if dir {
		flags = "OICI"
	}
	acl := "O:SYG:SYD:P(A;" + flags + ";FA;;;SY)(A;" + flags + ";FA;;;BA)"
	if serviceRights != "" {
		acl += "(A;" + flags + ";" + serviceRights + ";;;" + winServiceSID + ")"
	}
	return acl
}

func winSetSD(handle windows.Handle, text string, kind windows.SE_OBJECT_TYPE, full bool) error {
	sd, err := windows.SecurityDescriptorFromString(text)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	group, _, err := sd.Group()
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return errors.New("refusing permissive Windows descriptor")
	}
	flags := windows.SECURITY_INFORMATION(winReadSD)
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED != 0 {
		flags |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	} else {
		flags |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	}
	var sacl *windows.ACL
	if full {
		sacl, _, err = sd.SACL()
		if err != nil && !errors.Is(err, windows.ERROR_OBJECT_NOT_FOUND) {
			return err
		}
		flags |= windows.SACL_SECURITY_INFORMATION
		if control&windows.SE_SACL_PROTECTED != 0 {
			flags |= windows.PROTECTED_SACL_SECURITY_INFORMATION
		} else {
			flags |= windows.UNPROTECTED_SACL_SECURITY_INFORMATION
		}
	}
	return windows.SetSecurityInfo(handle, kind, flags, owner, group, dacl, sacl)
}

// Open every component without following reparse points and validate the
// system-owned ancestry. The daemon additionally pins its installation roots.
func winOpen(path string, access uint32) (*os.File, error) {
	if !winSafeAbsolute(path) {
		return nil, errors.New("noncanonical Windows path")
	}
	vol := filepath.VolumeName(path)
	parts := strings.Split(strings.TrimPrefix(path, vol+`\`), `\`)
	var ancestors []windows.Handle
	defer func() {
		for _, h := range ancestors {
			_ = windows.CloseHandle(h)
		}
	}()
	current := vol + `\`
	rootName, err := windows.UTF16PtrFromString(current)
	if err != nil {
		return nil, err
	}
	root, err := windows.CreateFile(rootName, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, err
	}
	ancestors = append(ancestors, root)
	if err := winAncestor(root); err != nil {
		return nil, err
	}
	for i, part := range parts {
		current = filepath.Join(current, part)
		p, err := windows.UTF16PtrFromString(current)
		if err != nil {
			return nil, err
		}
		rights := uint32(windows.FILE_READ_ATTRIBUTES | windows.FILE_READ_EA | windows.READ_CONTROL)
		if i == len(parts)-1 {
			rights |= access
		}
		h, err := windows.CreateFile(p, rights, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err != nil {
			return nil, err
		}
		var info windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(h, &info); err != nil {
			_ = windows.CloseHandle(h)
			return nil, err
		}
		if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_ENCRYPTED|windows.FILE_ATTRIBUTE_COMPRESSED|windows.FILE_ATTRIBUTE_SPARSE_FILE|windows.FILE_ATTRIBUTE_OFFLINE) != 0 {
			_ = windows.CloseHandle(h)
			return nil, errors.New("unsupported Windows reparse/extended file metadata")
		}
		if i == len(parts)-1 {
			if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 && info.NumberOfLinks != 1 {
				_ = windows.CloseHandle(h)
				return nil, errors.New("native Windows hardlinks require manual updates")
			}
			return os.NewFile(uintptr(h), path), nil
		}
		mutable := false
		for _, base := range []string{ConfigDir, DataDir} {
			mutable = mutable || strings.EqualFold(current, base) || strings.HasPrefix(strings.ToLower(current), strings.ToLower(base)+`\`)
		}
		if !mutable {
			if err := winAncestor(h); err != nil {
				_ = windows.CloseHandle(h)
				return nil, err
			}
		}
		ancestors = append(ancestors, h)
	}
	return nil, errors.New("native Windows root is not an update file")
}

func winAncestor(h windows.Handle) error {
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, winReadSD)
	if err != nil {
		return err
	}
	trusted := func(s string) bool {
		return s == "S-1-5-18" || s == "S-1-5-32-544" || s == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
	}
	owner, _, err := sd.Owner()
	if err != nil || !trusted(owner.String()) {
		return errors.New("native Windows installation ancestor is not system-owned")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.New("native Windows ancestor has a null DACL")
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("custom Windows ancestor ACE")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)) // #nosec G103 -- GetAce returns the native SID-at-offset layout
		if !trusted(sid.String()) && uint32(ace.Mask)&uint32(windows.FILE_WRITE_DATA|windows.FILE_WRITE_EA|windows.FILE_WRITE_ATTRIBUTES|0x40|windows.DELETE|windows.WRITE_DAC|windows.WRITE_OWNER|windows.GENERIC_WRITE|windows.GENERIC_ALL) != 0 {
			return errors.New("native Windows installation ancestor is writable by an untrusted account")
		}
	}
	return nil
}

func winSafeAbsolute(path string) bool {
	vol := filepath.VolumeName(path)
	if len(vol) != 2 || vol[1] != ':' || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path[2:], ":/") || len(path) <= 3 || !filepath.IsLocal(path[3:]) {
		return false
	}
	for _, c := range strings.Split(path[3:], `\`) {
		if c == "" || strings.TrimRight(c, ". ") != c {
			return false
		}
	}
	return true
}

func winTrusted(path string, rights uint32, private bool) error {
	f, err := winOpen(path, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, winReadSD)
	if err != nil {
		return err
	}
	if private {
		control, _, err := sd.Control()
		if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
			return errors.New("native Windows private state requires a protected DACL")
		}
	}
	return winCheckSD(sd, rights, private)
}

func winProtect(path, sddl string) error {
	f, err := winOpen(path, windows.WRITE_DAC|windows.WRITE_OWNER)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return winSetSD(windows.Handle(f.Fd()), sddl, windows.SE_FILE_OBJECT, false)
}
