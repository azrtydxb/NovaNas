//go:build linux

package main

import "syscall"

func rebootSystem() error {
	return syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
}
