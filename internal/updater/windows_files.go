//go:build windows

package updater

import (
	"archive/zip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type winMeta struct {
	Path       string `json:"path"`
	SDDL       string `json:"sddl"`
	Dir        bool   `json:"dir"`
	Attributes uint32 `json:"attributes"`
}

func winMetadata(f *os.File, relative string) (winMeta, error) {
	h := windows.Handle(f.Fd())
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return winMeta{}, fmt.Errorf("GetFileInformationByHandle: %w", err)
	}
	if info.FileAttributes & ^uint32(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_ARCHIVE|windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED) != 0 {
		return winMeta{}, errors.New("custom Windows file attributes require manual updates")
	}
	var ios windows.IO_STATUS_BLOCK
	ea := winAlignedNativeBuffer(4, 4)
	if err := windows.NtQueryInformationFile(h, &ios, &ea[0], uint32(len(ea)), windows.FileEaInformation); err != nil {
		return winMeta{}, winNTQueryError("NtQueryInformationFile(FileEaInformation)", err)
	}
	if binary.LittleEndian.Uint32(ea) != 0 {
		return winMeta{}, errors.New("NTFS extended attributes require manual updates")
	}
	dir := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	streams := winAlignedNativeBuffer(8192, 8)
	streamErr := windows.GetFileInformationByHandleEx(h, windows.FileStreamInfo, &streams[0], uint32(len(streams)))
	if dir {
		// NTFS directories have the ordinary $I30 index-allocation stream.
		// Some filesystems report no streams instead; accept EOF, but validate
		// any returned entry so directory ADS are not silently discarded.
		if errors.Is(streamErr, windows.ERROR_HANDLE_EOF) {
			// No streams were returned.
		} else if streamErr != nil {
			return winMeta{}, winStreamQueryError("directory", streamErr)
		} else if err := winValidateStreamInfo(streams[:], true); err != nil {
			return winMeta{}, err
		}
	} else {
		if streamErr != nil {
			return winMeta{}, winStreamQueryError("file", streamErr)
		}
		if err := winValidateStreamInfo(streams[:], false); err != nil {
			return winMeta{}, err
		}
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, winReadSD|windows.SACL_SECURITY_INFORMATION)
	if err != nil {
		return winMeta{}, fmt.Errorf("GetSecurityInfo(file metadata): %w", err)
	}
	rights := uint32(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE)
	if !strings.HasPrefix(strings.ToLower(f.Name()), strings.ToLower(BinDir)+`\`) {
		rights = 0x1301bf
	}
	if err := winCheckSD(sd, rights, false); err != nil {
		return winMeta{}, fmt.Errorf("validate Windows file descriptor: %w", err)
	}
	if _, err := windows.SecurityDescriptorFromString(sd.String()); err != nil {
		return winMeta{}, fmt.Errorf("parse Windows file descriptor: %w", err)
	}
	return winMeta{Path: relative, SDDL: sd.String(), Dir: dir, Attributes: info.FileAttributes}, nil
}

// Native file-information buffers have explicit ABI alignment requirements;
// byte slices alone guarantee only byte alignment.
func winAlignedNativeBuffer(size, alignment int) []byte {
	buffer := make([]byte, size+alignment-1)
	address := uintptr(unsafe.Pointer(&buffer[0])) // #nosec G103 -- inspect alignment without reconstructing the pointer
	offset := int((uintptr(alignment) - address%uintptr(alignment)) % uintptr(alignment))
	return buffer[offset : offset+size]
}

func winStreamQueryError(kind string, err error) error {
	var code windows.Errno
	if errors.As(err, &code) {
		return fmt.Errorf("query %s NTFS streams (Windows error 0x%08x): %w", kind, uint32(code), err)
	}
	return fmt.Errorf("query %s NTFS streams: %w", kind, err)
}

func winNTQueryError(call string, err error) error {
	var status windows.NTStatus
	if errors.As(err, &status) {
		return fmt.Errorf("%s, NTSTATUS 0x%08x: %w", call, uint32(status), err)
	}
	return fmt.Errorf("%s: %w", call, err)
}

// FileStreamInfo lists ordinary NTFS files as "::$DATA" and directories as
// ":$I30:$INDEX_ALLOCATION". A single exact default entry is expected; any
// additional entry may be an alternate stream and is rejected.
func winValidateStreamInfo(info []byte, dir bool) error {
	const streamInfoHeaderSize = 24
	if len(info) < streamInfoHeaderSize {
		return errors.New("truncated NTFS stream information")
	}
	next := binary.LittleEndian.Uint32(info[:4])
	nameBytes := binary.LittleEndian.Uint32(info[4:8])
	if nameBytes == 0 || nameBytes%2 != 0 || nameBytes > uint32(len(info)-streamInfoHeaderSize) {
		return errors.New("invalid NTFS stream name")
	}
	entrySize := uint32(streamInfoHeaderSize) + nameBytes
	if next != 0 {
		if next < entrySize || next%8 != 0 || next > uint32(len(info)) {
			return errors.New("invalid NTFS stream entry offset")
		}
		return errors.New("NTFS alternate streams require manual updates")
	}
	name := make([]uint16, nameBytes/2)
	for i := range name {
		name[i] = binary.LittleEndian.Uint16(info[streamInfoHeaderSize+i*2:])
	}
	want := "::$DATA"
	if dir {
		want = ":$I30:$INDEX_ALLOCATION"
	}
	if string(utf16.Decode(name)) != want {
		return errors.New("unexpected NTFS stream")
	}
	return nil
}

func winCopySnapshot(source, dest, relative string, limit int64) (winMeta, error) {
	in, err := winOpen(source, windows.GENERIC_READ|windows.ACCESS_SYSTEM_SECURITY)
	if err != nil {
		return winMeta{}, fmt.Errorf("open Windows snapshot source %q: %w", relative, err)
	}
	defer func() { _ = in.Close() }()
	meta, err := winMetadata(in, relative)
	if err != nil {
		return meta, fmt.Errorf("Windows snapshot metadata %q: %w", relative, err)
	}
	if meta.Dir {
		return meta, fmt.Errorf("Windows snapshot source %q is a directory", relative)
	}
	info, err := in.Stat()
	if err != nil || info.Size() > limit {
		return meta, errors.New("oversized Windows snapshot")
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) // #nosec G304 -- fixed private snapshot destination
	if err != nil {
		return meta, err
	}
	n, copyErr := io.Copy(out, io.LimitReader(in, limit+1))
	if n != info.Size() || n > limit {
		copyErr = errors.New("native Windows snapshot source changed")
	}
	return meta, errors.Join(copyErr, out.Sync(), out.Close())
}

func winSnapshotTree(source, dest string) ([]winMeta, error) {
	var entries []winMeta
	var total int64
	err := filepath.WalkDir(source, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if len(entries) >= 10000 {
			return errors.New("configuration exceeds entry limit")
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if !d.IsDir() {
			meta, err := winCopySnapshot(path, filepath.Join(dest, rel), rel, 64<<20)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
			if total > 64<<20 {
				return errors.New("configuration exceeds 64 MiB")
			}
			entries = append(entries, meta)
			return nil
		}
		f, err := winOpen(path, windows.ACCESS_SYSTEM_SECURITY)
		if err != nil {
			return err
		}
		meta, err := winMetadata(f, rel)
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("Windows snapshot directory metadata %q: %w", rel, err)
		}
		if err := os.Mkdir(filepath.Join(dest, rel), 0700); err != nil {
			return err
		}
		entries = append(entries, meta)
		return nil
	})
	return entries, err
}

func winSafeRelative(rel string) bool {
	if rel == "." {
		return true
	}
	if !filepath.IsLocal(rel) || filepath.Clean(rel) != rel || strings.ContainsAny(rel, ":/") || strings.HasSuffix(rel, `\`) {
		return false
	}
	for _, component := range strings.Split(rel, `\`) {
		if winReservedName(component) {
			return false
		}
	}
	return true
}

func winReplace(dest, source string, meta winMeta) error {
	if _, err := windows.SecurityDescriptorFromString(meta.SDDL); err != nil {
		return err
	}
	in, err := winOpen(source, windows.GENERIC_READ)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	tmp := dest + ".loom-update"
	if _, err := os.Lstat(tmp); err == nil {
		f, err := winOpen(tmp, 0)
		if err != nil {
			return err
		}
		_ = f.Close()
		if err := os.Remove(tmp); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600) // #nosec G304 -- fixed sibling under a pinned installation root, after quiescence
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	// Reopen with security access; no service-identity process may be running.
	err = errors.Join(err, out.Close())
	if err != nil {
		return err
	}
	secure, err := winOpen(tmp, windows.WRITE_DAC|windows.WRITE_OWNER|windows.ACCESS_SYSTEM_SECURITY)
	if err != nil {
		return err
	}
	err = winSetSD(windows.Handle(secure.Fd()), meta.SDDL, windows.SE_FILE_OBJECT, true)
	_ = secure.Close()
	if err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(dest)
	if err != nil {
		return err
	}
	if err := windows.SetFileAttributes(from, meta.Attributes); err != nil {
		return err
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return err
	}
	return winFlushVolume(dest)
}

func winRestoreTree(dest, backup string, entries []winMeta) error {
	// All service-SID processes and the entire no-breakaway job are stopped.
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	children, err := os.ReadDir(dest)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := root.RemoveAll(child.Name()); err != nil {
			return err
		}
	}
	for _, m := range entries {
		if !winSafeRelative(m.Path) {
			return errors.New("invalid private Windows journal path")
		}
		if m.Dir {
			if m.Path != "." {
				if err := os.Mkdir(filepath.Join(dest, m.Path), 0700); err != nil {
					return err
				}
			}
		} else if err := winReplace(filepath.Join(dest, m.Path), filepath.Join(backup, m.Path), m); err != nil {
			return err
		}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		m := entries[i]
		if !m.Dir {
			continue
		}
		f, err := winOpen(filepath.Join(dest, m.Path), windows.WRITE_DAC|windows.WRITE_OWNER|windows.ACCESS_SYSTEM_SECURITY)
		if err != nil {
			return err
		}
		err = winSetSD(windows.Handle(f.Fd()), m.SDDL, windows.SE_FILE_OBJECT, true)
		_ = f.Close()
		if err != nil {
			return err
		}
		path, err := windows.UTF16PtrFromString(filepath.Join(dest, m.Path))
		if err != nil {
			return err
		}
		attrs := m.Attributes &^ uint32(windows.FILE_ATTRIBUTE_DIRECTORY)
		if attrs == 0 {
			attrs = windows.FILE_ATTRIBUTE_NORMAL
		}
		if err := windows.SetFileAttributes(path, attrs); err != nil {
			return err
		}
	}
	return winFlushVolume(dest)
}

func winFlushVolume(path string) error {
	vol := filepath.VolumeName(path)
	if len(vol) != 2 || vol[1] != ':' {
		return errors.New("only local NTFS volumes are supported")
	}
	p, err := windows.UTF16PtrFromString(`\\.\` + vol)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	return windows.FlushFileBuffers(h)
}

func winAtomicJSON(dir, name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, name+".tmp")
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	if err := syncFile(tmp); err != nil {
		return err
	}
	a, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return err
	}
	return winFlushVolume(dir)
}
func winRemoveJournal(dir string) error {
	if err := os.Remove(filepath.Join(dir, "journal.json")); err != nil {
		return err
	}
	return winFlushVolume(dir)
}

func winExtractZIP(archive, dest string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer func() { _ = z.Close() }()
	found := false
	for _, f := range z.File {
		// Treat every other archive member as opaque, never extract its path.
		if !strings.EqualFold(f.Name, "loom.exe") {
			continue
		}
		if found || f.Name != "loom.exe" || !f.Mode().IsRegular() || f.UncompressedSize64 > 512<<20 || f.UncompressedSize64 == 0 {
			return errors.New("invalid/duplicate Windows executable member")
		}
		found = true
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) // #nosec G304 -- fixed private new-binary, never an archive member path
		if err != nil {
			_ = in.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, (512<<20)+1))
		if n < 0 || uint64(n) != f.UncompressedSize64 {
			copyErr = errors.New("native Windows executable exceeds declared size")
		}
		err = errors.Join(copyErr, in.Close(), out.Sync(), out.Close())
		if err != nil {
			return err
		}
	}
	if !found {
		return errors.New("native Windows archive has no loom.exe")
	}
	return nil
}
