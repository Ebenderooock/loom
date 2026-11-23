//go:build linux

package updater

import (
	"os"
	"strings"
	"testing"
)

func TestLinuxPreflightPreservesCustomUnits(t *testing.T) {
	if os.Getenv("LOOM_UPDATER_CONTAINER_TEST") != "1" || os.Geteuid() != 0 {
		t.Skip("isolated container integration only")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		t.Fatal("refusing fixed-path integration outside a container")
	}
	if err := os.MkdirAll("/etc/systemd/system", 0755); err != nil {
		t.Fatal(err)
	}
	path := "/etc/systemd/system/loom-updater.timer"
	custom := "# Custom administrator timer\n"
	if err := os.WriteFile(path, []byte(custom), 0644); err != nil {
		t.Fatal(err)
	} // #nosec G306 -- public unit, not a credential
	if err := preflightUnits(); err == nil || !strings.Contains(err.Error(), "customized") {
		t.Fatalf("custom unit accepted: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != custom {
		t.Fatal("preflight modified the custom unit", err)
	}
	if err := os.WriteFile(path, []byte(timerUnit), 0644); err != nil {
		t.Fatal(err)
	} // #nosec G306 -- public unit
	if err := os.Chmod(path, 0664); err != nil {
		t.Fatal(err)
	} // #nosec G302 -- deliberately unsafe fixture; must be rejected
	if err := preflightUnits(); err == nil {
		t.Fatal("group-writable unit accepted")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0664 {
		t.Fatal("preflight changed existing permissions", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := preflightUnits(); err != nil {
		t.Fatal("canonical units rejected", err)
	}
}
