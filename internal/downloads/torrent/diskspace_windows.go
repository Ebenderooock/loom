//go:build windows

package torrent

import (
	"syscall"
	"unsafe"
)

func diskFreeSpace(path string) (int64, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return -1, err
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	ret, _, callErr := syscall.NewLazyDLL("kernel32.dll").
		NewProc("GetDiskFreeSpaceExW").
		Call(
			uintptr(unsafe.Pointer(pathPtr)),
			uintptr(unsafe.Pointer(&freeBytesAvailable)),
			uintptr(unsafe.Pointer(&totalBytes)),
			uintptr(unsafe.Pointer(&totalFreeBytes)),
		)
	if ret == 0 {
		return -1, callErr
	}
	return int64(freeBytesAvailable), nil
}
