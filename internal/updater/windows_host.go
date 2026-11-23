//go:build windows

package updater

import (
	"errors"
	"os"

	"golang.org/x/sys/windows/registry"
)

func winNativeHost() error {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return errors.New("native updates are unavailable in Kubernetes; use orchestration updates")
	}
	control, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control`, registry.QUERY_VALUE)
	if err != nil {
		return errors.New("cannot establish native Windows host")
	}
	_, _, err = control.GetIntegerValue("ContainerType")
	_ = control.Close()
	if err == nil {
		return errors.New("native updates are unavailable in Windows containers; use image updates")
	}
	if !errors.Is(err, registry.ErrNotExist) {
		return errors.New("cannot exclude Windows container type")
	}
	for _, name := range []string{`SYSTEM\CurrentControlSet\Control\ContainerType`, `SYSTEM\CurrentControlSet\Services\cexecsvc`} {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, name, registry.READ)
		if err == nil {
			_ = key.Close()
			return errors.New("native updates are unavailable in Windows containers; use image updates")
		}
		if !errors.Is(err, registry.ErrNotExist) {
			return errors.New("cannot exclude Windows container host markers")
		}
	}
	return nil
}
