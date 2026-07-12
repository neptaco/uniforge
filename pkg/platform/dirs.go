package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

const appName = "uniforge"

// RuntimeDir returns the directory for transient runtime files (daemon.json, daemon.pid, daemon.sock).
// - Linux:   $XDG_RUNTIME_DIR/uniforge  (fallback: /tmp/uniforge-$UID)
// - macOS:   $TMPDIR/uniforge
// - Windows: %LOCALAPPDATA%\uniforge
func RuntimeDir() (string, error) {
	switch runtime.GOOS {
	case "linux":
		if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
			return filepath.Join(dir, appName), nil
		}
		return filepath.Join(os.TempDir(), appName+"-"+uidString()), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Caches", appName), nil
	default: // windows
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, appName), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "."+appName), nil
	}
}

// StateDir returns the directory for persistent state files (logs).
// - Linux:   $XDG_STATE_HOME/uniforge  (fallback: ~/.local/state/uniforge)
// - macOS:   ~/Library/Logs/uniforge
// - Windows: %LOCALAPPDATA%\uniforge\logs
func StateDir() (string, error) {
	switch runtime.GOOS {
	case "linux":
		if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
			return filepath.Join(dir, appName), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "state", appName), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Logs", appName), nil
	default: // windows
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, appName, "logs"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "."+appName, "logs"), nil
	}
}
