package testutil

import (
	"os"

	"golang.org/x/sys/windows"
)

// MarkSparse allows a test fixture to grow without allocating its full payload.
func MarkSparse(f *os.File) error {
	// NTFS otherwise allocates and zero-fills the full fixture on extension.
	var returned uint32
	return windows.DeviceIoControl(windows.Handle(f.Fd()), windows.FSCTL_SET_SPARSE,
		nil, 0, nil, 0, &returned, nil)
}
