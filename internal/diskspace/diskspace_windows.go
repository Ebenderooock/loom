//go:build windows

package diskspace

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func volumeID(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("absolute volume path: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve volume path: %w", err)
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", fmt.Errorf("encode volume path: %w", err)
	}
	root := make([]uint16, 32768)
	if err := windows.GetVolumePathName(p, &root[0], uint32(len(root))); err != nil {
		return "", fmt.Errorf("get volume path for %q: %w", path, err)
	}
	var serial uint32
	if err := windows.GetVolumeInformation(&root[0], nil, 0, &serial, nil, nil, nil, 0); err != nil {
		return "", fmt.Errorf("get volume information for %q: %w", path, err)
	}
	// A volume GUID identifies the same local volume through different mount
	// points. Network shares may not have one, so use their root path instead.
	volume := make([]uint16, 32768)
	rootName := windows.UTF16ToString(root)
	if err := windows.GetVolumeNameForVolumeMountPoint(&root[0], &volume[0], uint32(len(volume))); err == nil {
		rootName = windows.UTF16ToString(volume)
	}
	return fmt.Sprintf("volume:%08x:%s", serial, strings.ToLower(rootName)), nil
}

func get(path string) (total, free uint64, err error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeBytesAvailable, &totalBytes, &totalFreeBytes); err != nil {
		return 0, 0, err
	}
	return totalBytes, freeBytesAvailable, nil
}
