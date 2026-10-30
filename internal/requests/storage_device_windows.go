//go:build windows

package requests

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

func sameStorageDevice(rootPath, filePath string, _, _ os.FileInfo) bool {
	volume := func(path string) (string, error) {
		p, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return "", err
		}
		buffer := make([]uint16, 32768)
		if err := windows.GetVolumePathName(p, &buffer[0], uint32(len(buffer))); err != nil {
			return "", err
		}
		return windows.UTF16ToString(buffer), nil
	}
	rootVolume, rootErr := volume(rootPath)
	fileVolume, fileErr := volume(filePath)
	return rootErr == nil && fileErr == nil && strings.EqualFold(rootVolume, fileVolume)
}
