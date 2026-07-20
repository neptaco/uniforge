package unity

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/neptaco/uniforge/pkg/platform"
)

// GetManagedEditorLogPath returns UniForge's deterministic per-project Editor log path.
func GetManagedEditorLogPath(projectPath string) (string, error) {
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute project path: %w", err)
	}

	stateDir, err := platform.StateDir()
	if err != nil {
		return "", fmt.Errorf("failed to get state directory: %w", err)
	}

	return buildEditorLogPath(stateDir, absProjectPath), nil
}

func buildEditorLogPath(stateDir, projectPath string) string {
	cleanProjectPath := filepath.Clean(projectPath)
	projectName := sanitizeProjectName(filepath.Base(cleanProjectPath))
	hashInput := cleanProjectPath
	if runtime.GOOS == "windows" {
		hashInput = strings.ToLower(hashInput)
	}
	pathHash := sha256.Sum256([]byte(hashInput))
	fileName := fmt.Sprintf("%s-%x.log", projectName, pathHash[:4])
	return filepath.Join(stateDir, "editor-logs", fileName)
}

func sanitizeProjectName(name string) string {
	var sanitized strings.Builder
	separatorPending := false
	for _, character := range name {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '.' || character == '-' || character == '_' {
			if separatorPending && sanitized.Len() > 0 {
				sanitized.WriteByte('_')
			}
			sanitized.WriteRune(character)
			separatorPending = false
			continue
		}
		separatorPending = true
	}

	result := strings.Trim(sanitized.String(), ".")
	if result == "" {
		return "project"
	}
	if isWindowsReservedFilename(result) {
		return "_" + result
	}
	return result
}

func isWindowsReservedFilename(name string) bool {
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) {
		return base[3] >= '1' && base[3] <= '9'
	}
	return false
}

// GetEditorLogPath returns the platform-specific path to Unity Editor log
func GetEditorLogPath() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(home, "Library", "Logs", "Unity", "Editor.log"), nil

	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			return "", fmt.Errorf("LOCALAPPDATA environment variable not set")
		}
		return filepath.Join(localAppData, "Unity", "Editor", "Editor.log"), nil

	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(home, ".config", "unity3d", "Editor.log"), nil

	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}
