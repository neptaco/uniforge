//go:build !windows

package unity

import (
	"errors"
	"syscall"
)

func terminateProcessByPID(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func killProcessByPID(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}

func isProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	// EPERM means the process exists but belongs to another user.
	return errors.Is(err, syscall.EPERM)
}
