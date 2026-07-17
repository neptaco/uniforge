//go:build windows

package unity

import "os"

// Windows has no portable graceful signal, so both steps terminate forcefully.
func terminateProcessByPID(pid int) error {
	return killProcessByPID(pid)
}

func killProcessByPID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

// isProcessAlive is best-effort: on Windows FindProcess opens a handle to the
// target, which fails once the process is gone.
func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = process.Release()
	return true
}
