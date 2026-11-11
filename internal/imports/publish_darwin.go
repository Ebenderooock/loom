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
