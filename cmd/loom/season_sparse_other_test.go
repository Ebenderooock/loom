//go:build !windows

package main

import "os"

func markSeasonFileSparse(_ *os.File) error {
	return nil
}
