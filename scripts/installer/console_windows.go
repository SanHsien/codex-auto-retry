//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

const createNewConsole = 0x00000010

// useHiddenConsole starts the child in its own hidden console so that
// scripts which hide "their" console window cannot hide the caller's.
func useHiddenConsole(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNewConsole}
}
