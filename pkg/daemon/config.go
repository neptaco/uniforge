package daemon

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Config configures a daemon instance.
type Config struct {
	// Name is the application name, used for directory names and pipe names.
	// Must not be empty.
	Name string

	// RuntimeDir overrides the default runtime directory for transient files
	// (info, PID, lock, socket). If empty, uses OS defaults:
	//   - macOS:   ~/Library/Caches/{Name}
	//   - Linux:   $XDG_RUNTIME_DIR/{Name} (fallback: /tmp/{Name}-$UID)
	//   - Windows: %LOCALAPPDATA%\{Name}
	RuntimeDir string

	// StateDir overrides the default state directory for persistent files (logs).
	// If empty, uses OS defaults:
	//   - macOS:   ~/Library/Logs/{Name}
	//   - Linux:   $XDG_STATE_HOME/{Name} (fallback: ~/.local/state/{Name})
	//   - Windows: %LOCALAPPDATA%\{Name}\logs
	StateDir string
}

// Validate checks that the configuration is valid.
func (c Config) Validate() error {
	if c.Name == "" {
		return errors.New("daemon: config name must not be empty")
	}
	return nil
}

func (c Config) runtimeDir() (string, error) {
	if c.RuntimeDir != "" {
		return c.RuntimeDir, nil
	}
	return defaultRuntimeDir(c.Name)
}

func (c Config) stateDir() (string, error) {
	if c.StateDir != "" {
		return c.StateDir, nil
	}
	return defaultStateDir(c.Name)
}

func (c Config) infoPath() (string, error) {
	dir, err := c.runtimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daemon.json"), nil
}

func (c Config) lockPath() (string, error) {
	dir, err := c.runtimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daemon.lock"), nil
}

func (c Config) lifecycleLockPath() (string, error) {
	dir, err := c.runtimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daemon.lifecycle.lock"), nil
}

func (c Config) logPath() (string, error) {
	dir, err := c.stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daemon.log"), nil
}

// resolveEndpoint returns the default IPC endpoint for this platform.
func (c Config) resolveEndpoint() (Info, error) {
	dir, err := c.runtimeDir()
	if err != nil {
		return Info{}, err
	}

	if runtime.GOOS == "windows" {
		hash := sha1.Sum([]byte(dir))
		return Info{
			Transport: TransportNamedPipe,
			Endpoint:  fmt.Sprintf(`\\.\pipe\%s-%x`, c.Name, hash[:6]),
		}, nil
	}

	return Info{
		Transport: TransportUnix,
		Endpoint:  filepath.Join(dir, "daemon.sock"),
	}, nil
}

// defaultRuntimeDir returns the platform-specific runtime directory.
func defaultRuntimeDir(name string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
			return filepath.Join(dir, name), nil
		}
		return filepath.Join(os.TempDir(), name+"-"+uidString()), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Caches", name), nil
	default: // windows
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, name), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "."+name), nil
	}
}

// defaultStateDir returns the platform-specific state directory (for logs).
func defaultStateDir(name string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
			return filepath.Join(dir, name), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "state", name), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Logs", name), nil
	default: // windows
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, name, "logs"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "."+name, "logs"), nil
	}
}
