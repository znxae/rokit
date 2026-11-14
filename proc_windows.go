package main

import (
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// hideWindow stops child processes from flashing a console window, and makes
// cancelling kill the whole tree (yt-dlp spawns ffmpeg to merge).
func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		k := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(c.Process.Pid))
		k.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		_ = k.Run()
		return c.Process.Kill()
	}
	c.WaitDelay = 5 * time.Second
}
