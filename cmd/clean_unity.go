package cmd

import (
	"fmt"
	"strings"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var cleanUnityCmd = &cobra.Command{
	Use:   "unity [project]",
	Short: "Deprecated alias for clean",
	Long: `Clean stale Unity project runtime files.

The command only touches explicitly selected targets and verifies that the
Unity lockfile is not held by a running Editor before removing files.

Supported targets:
  lockfile  Temp/UnityLockfile

Examples:
  uniforge clean /path/to/project --target lockfile
  uniforge clean --target lockfile --dry-run`,
	Args:       cobra.MaximumNArgs(1),
	RunE:       runClean,
	Deprecated: "use `uniforge clean [project] --target ...` instead",
	Hidden:     true,
}

func init() {
	cleanCmd.AddCommand(cleanUnityCmd)
	addCleanFlags(cleanUnityCmd)
}

func addCleanFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray("target", nil, "Clean target to apply (repeatable): lockfile")
	cmd.Flags().Bool("dry-run", false, "Show what would be removed without deleting files")
}

func runClean(cmd *cobra.Command, args []string) error {
	project, err := resolveLoadedProjectArg(args)
	if err != nil {
		return err
	}

	targetValues, err := cmd.Flags().GetStringArray("target")
	if err != nil {
		return err
	}
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return err
	}
	targets, err := parseCleanUnityTargets(targetValues)
	if err != nil {
		return err
	}

	if dryRun {
		ui.Info("Checking clean targets for project: %s", project.Path)
	} else {
		ui.Info("Cleaning project: %s", project.Path)
	}

	result, err := unity.CleanUnityProject(unity.CleanOptions{
		ProjectPath: project.Path,
		Targets:     targets,
		DryRun:      dryRun,
	})

	if result != nil {
		printCleanUnityResult(result)
	}
	if err != nil {
		return fmt.Errorf("clean failed: %w", err)
	}

	ui.Success("Clean completed")
	return nil
}

func parseCleanUnityTargets(values []string) ([]unity.CleanTarget, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one --target is required (supported: %s)", supportedCleanUnityTargetsText())
	}

	targets := make([]unity.CleanTarget, 0, len(values))
	for _, value := range values {
		target := unity.CleanTarget(strings.TrimSpace(strings.ToLower(value)))
		if !isSupportedCleanUnityTarget(target) {
			return nil, fmt.Errorf("unsupported clean target %q (supported: %s)", value, supportedCleanUnityTargetsText())
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func isSupportedCleanUnityTarget(target unity.CleanTarget) bool {
	for _, supported := range unity.SupportedCleanTargets() {
		if target == supported {
			return true
		}
	}
	return false
}

func supportedCleanUnityTargetsText() string {
	targets := unity.SupportedCleanTargets()
	values := make([]string, len(targets))
	for i, target := range targets {
		values[i] = string(target)
	}
	return strings.Join(values, ", ")
}

func printCleanUnityResult(result *unity.CleanResult) {
	for _, item := range result.Items {
		line := fmt.Sprintf("%s: %s", item.Target, item.Path)
		if item.Message != "" {
			line = fmt.Sprintf("%s (%s)", line, item.Message)
		}

		switch item.Status {
		case unity.CleanItemRemoved:
			ui.Success("%s", line)
		case unity.CleanItemWouldClean:
			ui.Info("%s", line)
		case unity.CleanItemMissing:
			ui.Muted("%s", line)
		default:
			ui.Warn("%s", line)
		}
	}
}
