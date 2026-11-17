package imports

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func publishCopy(staged, dst string) error {
	err := unix.RenameatxNp(unix.AT_FDCWD, staged, unix.AT_FDCWD, dst, unix.RENAME_EXCL)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOTSUP) {
		return os.Link(staged, dst)
	}
	return err
}

func publishReplacement(staged, dst string) error {
	// Ordinary rename can briefly expose ENOENT to concurrent APFS readers.
	// Swap keeps both names present; the old file stays in the staging directory.
	if err := unix.RenameatxNp(unix.AT_FDCWD, staged, unix.AT_FDCWD, dst, unix.RENAME_SWAP); err != nil {
		return &os.LinkError{Op: "rename", Old: staged, New: dst, Err: err}
	}
	return nil
}
