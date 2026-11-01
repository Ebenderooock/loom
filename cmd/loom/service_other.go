//go:build !windows

package main

import "fmt"

func cmdService(_ []string) error {
	return fmt.Errorf("service is Windows-only; use 'serve' with systemd or launchd")
}
