package imports

import "golang.org/x/sys/windows"

func publishCopy(staged, dst string) error {
	from, err := windows.UTF16PtrFromString(staged)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	return windows.MoveFile(from, to)
}
