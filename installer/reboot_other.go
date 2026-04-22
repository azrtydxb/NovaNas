//go:build !linux

package main

import "fmt"

// rebootSystem is a no-op on non-Linux platforms. The installer is only ever
// run inside the Linux live ISO; this stub exists so the binary builds on
// developer workstations.
func rebootSystem() error {
	return fmt.Errorf("reboot not supported on this platform")
}
