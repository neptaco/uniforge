package unity

import (
	"fmt"
	"os"
	"path/filepath"
)

type CleanTarget string

const (
	CleanTargetLockfile CleanTarget = "lockfile"
)

type CleanOptions struct {
	ProjectPath string
	Targets     []CleanTarget
	DryRun      bool

	probeLockfile unityLockfileProbe
}

type CleanItemStatus string

const (
	CleanItemMissing    CleanItemStatus = "missing"
	CleanItemRemoved    CleanItemStatus = "removed"
	CleanItemSkipped    CleanItemStatus = "skipped"
	CleanItemWouldClean CleanItemStatus = "would-clean"
)

type CleanItem struct {
	Target  CleanTarget
	Path    string
	Status  CleanItemStatus
	Message string
}

type CleanResult struct {
	ProjectPath string
	Items       []CleanItem
}

func CleanUnityProject(options CleanOptions) (*CleanResult, error) {
	absProjectPath, err := filepath.Abs(options.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}
	if len(options.Targets) == 0 {
		return nil, fmt.Errorf("at least one clean target is required")
	}

	result := &CleanResult{
		ProjectPath: absProjectPath,
		Items:       make([]CleanItem, 0, len(options.Targets)),
	}

	for _, target := range options.Targets {
		switch target {
		case CleanTargetLockfile:
			item, err := cleanUnityLockfile(absProjectPath, options)
			result.Items = append(result.Items, item)
			if err != nil {
				return result, err
			}
		default:
			return result, fmt.Errorf("unsupported clean target: %s", target)
		}
	}

	return result, nil
}

func cleanUnityLockfile(absProjectPath string, options CleanOptions) (CleanItem, error) {
	lockfile := filepath.Join(absProjectPath, "Temp", "UnityLockfile")
	item := CleanItem{
		Target: CleanTargetLockfile,
		Path:   lockfile,
	}

	if _, err := os.Stat(lockfile); err != nil {
		if os.IsNotExist(err) {
			item.Status = CleanItemMissing
			item.Message = "lockfile does not exist"
			return item, nil
		}
		item.Status = CleanItemSkipped
		item.Message = "failed to inspect lockfile"
		return item, fmt.Errorf("failed to inspect %s: %w", lockfile, err)
	}

	probeLockfile := options.probeLockfile
	if probeLockfile == nil {
		probeLockfile = probeUnityLockfile
	}
	held, err := probeLockfile(lockfile)
	if err != nil {
		item.Status = CleanItemSkipped
		item.Message = "failed to inspect Unity lock state"
		return item, fmt.Errorf("failed to inspect Unity lock state before removing %s: %w", lockfile, err)
	}
	if held {
		item.Status = CleanItemSkipped
		item.Message = "lockfile is held by a running Unity Editor"
		return item, fmt.Errorf("refusing to remove %s because its OS lock is held", lockfile)
	}

	if options.DryRun {
		item.Status = CleanItemWouldClean
		item.Message = "would remove stale lockfile"
		return item, nil
	}

	if err := os.Remove(lockfile); err != nil {
		item.Status = CleanItemSkipped
		item.Message = "failed to remove lockfile"
		return item, fmt.Errorf("failed to remove %s: %w", lockfile, err)
	}

	item.Status = CleanItemRemoved
	item.Message = "removed stale lockfile"
	return item, nil
}

func SupportedCleanTargets() []CleanTarget {
	return []CleanTarget{CleanTargetLockfile}
}
