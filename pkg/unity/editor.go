package unity

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/ui"
)

type Editor struct {
	Version string
	Path    string
}

type unityProcessFinder func(projectPath string) (int, error)

const unityProcessExitPollInterval = 200 * time.Millisecond

func NewEditor(version string) *Editor {
	return &Editor{
		Version: version,
	}
}

func (e *Editor) GetPath() (string, error) {
	if e.Path != "" {
		return e.Path, nil
	}

	hubClient := hub.NewClient()

	// First, try to find editor via install path (faster, doesn't require Hub CLI)
	installPath, err := hubClient.GetInstallPath()
	if err == nil && installPath != "" {
		editorDir := filepath.Join(installPath, e.Version)
		execPath := e.getExecutablePath(editorDir)
		if fileExists(execPath) {
			ui.Debug("Found Unity Editor via install path", "version", e.Version, "path", execPath)
			e.Path = execPath
			return e.Path, nil
		}
	}

	// Fallback: try Hub CLI to list installed editors
	editors, err := hubClient.ListInstalledEditors()
	if err != nil {
		return "", fmt.Errorf("unity editor %s not found. install path: %s, hub error: %w", e.Version, installPath, err)
	}

	for _, editor := range editors {
		if editor.Version == e.Version {
			e.Path = e.getExecutablePath(editor.Path)
			return e.Path, nil
		}
	}

	return "", fmt.Errorf("unity editor %s not found, please install it using: uniforge editor install %s", e.Version, e.Version)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (e *Editor) getExecutablePath(installPath string) string {
	return resolveEditorExecutablePath(runtime.GOOS, installPath)
}

func resolveEditorExecutablePath(goos, installPath string) string {
	installPath = filepath.Clean(installPath)

	switch goos {
	case "darwin":
		executableSuffix := filepath.Join("Unity.app", "Contents", "MacOS", "Unity")
		if strings.HasSuffix(installPath, executableSuffix) {
			return installPath
		}
		// Unity Hub may return path ending with .app (e.g., /path/to/Unity.app)
		if strings.HasSuffix(installPath, ".app") {
			return filepath.Join(installPath, "Contents", "MacOS", "Unity")
		}
		return filepath.Join(installPath, "Unity.app", "Contents", "MacOS", "Unity")
	case "windows":
		// Unity Hub already returns the full path to Unity.exe, so just return it as-is
		if strings.EqualFold(filepath.Ext(installPath), ".exe") {
			return installPath
		}
		return filepath.Join(installPath, "Editor", "Unity.exe")
	case "linux":
		if strings.HasSuffix(installPath, filepath.Join("Editor", "Unity")) {
			return installPath
		}
		return filepath.Join(installPath, "Editor", "Unity")
	default:
		return filepath.Join(installPath, "Unity")
	}
}

func (e *Editor) Exists() bool {
	path, err := e.GetPath()
	if err != nil {
		return false
	}

	_, err = os.Stat(path)
	return err == nil
}

// Open starts the Unity Editor with the specified project in GUI mode
func (e *Editor) Open(projectPath string) error {
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if Unity Editor is already running for this project
	if err := e.CheckNotRunning(absProjectPath); err != nil {
		return err
	}

	editorPath, err := e.GetPath()
	if err != nil {
		return fmt.Errorf("failed to get Unity Editor path: %w", err)
	}

	args := []string{"-projectPath", absProjectPath}

	ui.Debug("Opening Unity Editor", "path", editorPath, "args", strings.Join(args, " "))

	cmd := exec.Command(editorPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Unity Editor: %w", err)
	}

	ui.Debug("Unity Editor started", "pid", cmd.Process.Pid)
	return nil
}

// Close terminates the Unity Editor process for the specified project
func (e *Editor) Close(projectPath string, force bool) error {
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	pid, err := e.findUnityProcess(absProjectPath)
	if err != nil {
		return err
	}

	if pid == 0 {
		return fmt.Errorf("no Unity Editor process found for project: %s", absProjectPath)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	if force {
		ui.Debug("Force killing Unity Editor process", "pid", pid)
		if err := process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		if err := waitForUnityProcessExit(absProjectPath, 10*time.Second, unityProcessExitPollInterval, e.findUnityProcess); err != nil {
			return err
		}
	} else {
		ui.Debug("Terminating Unity Editor process", "pid", pid)
		if err := process.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to terminate process: %w", err)
		}

		if err := waitForUnityProcessExit(absProjectPath, 10*time.Second, unityProcessExitPollInterval, e.findUnityProcess); err == nil {
			ui.Debug("Unity Editor terminated gracefully")
		} else {
			ui.Warn("Grace period expired, force killing...")
			if err := process.Kill(); err != nil {
				return fmt.Errorf("failed to kill process: %w", err)
			}
			if err := waitForUnityProcessExit(absProjectPath, 5*time.Second, unityProcessExitPollInterval, e.findUnityProcess); err != nil {
				return err
			}
		}
	}

	return nil
}

// CheckNotRunning returns an error if Unity Editor is already running for the project
func (e *Editor) CheckNotRunning(projectPath string) error {
	pid, err := e.findUnityProcess(projectPath)
	if err != nil {
		return fmt.Errorf("failed to check Unity process: %w", err)
	}
	if pid != 0 {
		return fmt.Errorf("unity Editor is already running for this project (PID: %d)", pid)
	}
	return nil
}

// findUnityProcess finds the Unity Editor process for the specified project
func (e *Editor) findUnityProcess(projectPath string) (int, error) {
	switch runtime.GOOS {
	case "darwin":
		return e.findUnityProcessDarwin(projectPath)
	case "windows":
		return e.findUnityProcessWindows(projectPath)
	case "linux":
		return e.findUnityProcessLinux(projectPath)
	default:
		return 0, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func (e *Editor) findUnityProcessDarwin(projectPath string) (int, error) {
	cmd := exec.Command("ps", "-ax", "-o", "pid=,command=")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to list processes: %w", err)
	}

	return findUnityProcessFromPSOutput(string(output), projectPath), nil
}

func (e *Editor) findUnityProcessWindows(projectPath string) (int, error) {
	// Use wmic to find Unity process
	cmd := exec.Command("wmic", "process", "where", "name='Unity.exe'", "get", "ProcessId,CommandLine", "/format:csv")
	output, err := cmd.Output()
	if err != nil {
		return 0, nil
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, projectPath) {
			fields := strings.Split(strings.TrimSpace(line), ",")
			if len(fields) >= 3 {
				var pid int
				if _, err := fmt.Sscanf(fields[len(fields)-1], "%d", &pid); err == nil {
					return pid, nil
				}
			}
		}
	}

	return 0, nil
}

func (e *Editor) findUnityProcessLinux(projectPath string) (int, error) {
	return e.findUnityProcessDarwin(projectPath)
}

func findUnityProcessFromPSOutput(output, projectPath string) int {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(line), "unity") {
			continue
		}
		if !strings.Contains(line, projectPath) {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		var pid int
		if _, err := fmt.Sscanf(fields[0], "%d", &pid); err == nil {
			return pid
		}
	}

	return 0
}

func waitForUnityProcessExit(
	projectPath string,
	timeout time.Duration,
	pollInterval time.Duration,
	findProcess unityProcessFinder,
) error {
	if pollInterval <= 0 {
		pollInterval = unityProcessExitPollInterval
	}

	deadline := time.Now().Add(timeout)
	for {
		pid, err := findProcess(projectPath)
		if err != nil {
			return fmt.Errorf("failed to verify Unity process exit: %w", err)
		}
		if pid == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("unity Editor is still running for project: %s", projectPath)
		}

		time.Sleep(pollInterval)
	}
}
