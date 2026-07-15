package cmd

import (
	"fmt"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var doctorUnityFix bool

var doctorUnityCmd = &cobra.Command{
	Use:   "unity [project]",
	Short: "Diagnose Unity runtime files and helper processes",
	Long: `Diagnose transient Unity state that can block Editor startup or batch mode.

By default this command is read-only. With --fix it removes stale project
runtime files and stops orphan licensing clients only when no Unity Editor is
running. Use clean unity when you explicitly want to remove a selected file.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDoctorUnity,
}

func init() {
	doctorCmd.AddCommand(doctorUnityCmd)
	doctorUnityCmd.Flags().BoolVar(&doctorUnityFix, "fix", false, "repair stale Unity runtime state")
}

func runDoctorUnity(cmd *cobra.Command, args []string) error {
	project, err := resolveLoadedProjectArg(args)
	if err != nil {
		return err
	}

	ui.Info("Checking Unity runtime: %s", project.Path)
	result, err := unity.NewRuntimeDoctor().Check(project.Path, doctorUnityFix)
	if err != nil {
		if result != nil {
			printRuntimeDoctorResult(result)
		}
		return fmt.Errorf("unity runtime doctor failed: %w", err)
	}
	printRuntimeDoctorResult(result)

	if result.HasUnfixedBlockingIssues() {
		if !doctorUnityFix && result.HasFixableIssues() {
			ui.Muted("Run `uniforge doctor unity --fix` for this project to repair safe-to-fix stale state")
		}
		return fmt.Errorf("unity runtime has blocking issue(s)")
	}
	if !result.HasIssues() {
		ui.Success("Unity runtime is clean")
	} else if len(result.Fixes) > 0 {
		ui.Success("Unity runtime issues fixed")
	}
	return nil
}

func printRuntimeDoctorResult(result *unity.RuntimeDoctorResult) {
	if result.EditorPID != 0 {
		ui.Muted("Unity Editor PID: %d", result.EditorPID)
	}
	for _, fix := range result.Fixes {
		ui.Success("%s", formatRuntimeDetail(fix.Message, fix.Path, fix.PID))
	}
	for _, issue := range result.Issues {
		if issue.Fixed {
			continue
		}
		detail := formatRuntimeDetail(issue.Message, issue.Path, issue.PID)
		if issue.Blocking {
			ui.Warn("%s", detail)
		} else {
			ui.Muted("%s", detail)
		}
	}
}

func formatRuntimeDetail(message, path string, pid int) string {
	detail := message
	if path != "" {
		detail = fmt.Sprintf("%s: %s", detail, path)
	}
	if pid != 0 {
		detail = fmt.Sprintf("%s (pid %d)", detail, pid)
	}
	return detail
}
