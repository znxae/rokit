package main

import (
	"os/exec"
	"syscall"
)

// hideWindow stops child processes from flashing a console window.
func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
