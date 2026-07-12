package unity

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

const listUnityProcessesPowerShell = `$ErrorActionPreference = 'Stop'
$OutputEncoding = [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$processes = @(Get-CimInstance Win32_Process -Filter "Name = 'Unity.exe'" | Select-Object ProcessId, CommandLine)
ConvertTo-Json -InputObject $processes -Compress`

type windowsProcess struct {
	ProcessID   int    `json:"ProcessId"`
	CommandLine string `json:"CommandLine"`
}

func (e *Editor) findUnityProcessWindows(projectPath string) (int, error) {
	cmd := exec.Command(
		"powershell.exe",
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-Command", listUnityProcessesPowerShell,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to list Unity processes with PowerShell/CIM: %w", err)
	}

	pid, err := findUnityProcessFromWindowsJSON(output, projectPath)
	if err != nil {
		return 0, fmt.Errorf("parse PowerShell/CIM process output: %w", err)
	}
	return pid, nil
}

func findUnityProcessFromWindowsJSON(output []byte, projectPath string) (int, error) {
	if strings.TrimSpace(projectPath) == "" {
		return 0, fmt.Errorf("project path is empty")
	}

	var processes []windowsProcess
	if err := json.Unmarshal(output, &processes); err != nil {
		return 0, err
	}

	normalizedProjectPath := normalizeWindowsPath(projectPath)
	for _, process := range processes {
		if process.ProcessID <= 0 || process.CommandLine == "" {
			continue
		}
		if strings.Contains(normalizeWindowsPath(process.CommandLine), normalizedProjectPath) {
			return process.ProcessID, nil
		}
	}
	return 0, nil
}

func normalizeWindowsPath(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, "/", `\`))
}
