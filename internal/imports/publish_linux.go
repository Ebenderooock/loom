package imports

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func publishCopy(staged, dst string) error {
	err := unix.Renameat2(unix.AT_FDCWD, staged, unix.AT_FDCWD, dst, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		return os.Link(staged, dst)
	}
	return err
}
