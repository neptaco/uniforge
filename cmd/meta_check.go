package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var (
	metaCheckFix   bool
	metaCheckForce bool
)

var metaCheckCmd = &cobra.Command{
	Use:   "check [project]",
	Short: "Check .meta file integrity",
	Long: `Check Unity project for .meta file integrity issues.

This command checks for:
  - Missing .meta files (Error): Assets without corresponding .meta files
  - Orphan .meta files (Warning): .meta files without corresponding assets
  - Duplicate GUIDs (Error): Multiple .meta files with the same GUID

Use --fix to automatically resolve issues:
  - Generate missing .meta files
  - Remove orphan .meta files

Examples:
  uniforge meta check
  uniforge meta check /path/to/project
  uniforge meta check --fix
  uniforge meta check --fix --force`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMetaCheck,
}

func init() {
	metaCmd.AddCommand(metaCheckCmd)

	metaCheckCmd.Flags().BoolVar(&metaCheckFix, "fix", false, "Generate missing .meta files and remove orphan .meta files")
	metaCheckCmd.Flags().BoolVar(&metaCheckForce, "force", false, "Skip confirmation when using --fix (for CI)")
}

func runMetaCheck(cmd *cobra.Command, args []string) error {
	project, err := resolveLoadedProjectArg(args)
	if err != nil {
		return err
	}

	ui.Info("Checking .meta files in: %s", project.Path)

	checker := unity.NewMetaChecker(project)

	result, err := ui.WithSpinner("Scanning project...", func() (*unity.MetaCheckResult, error) {
		return checker.Check()
	})
	if err != nil {
		return fmt.Errorf("check failed: %w", err)
	}

	hasOutput := false

	// Missing meta files (Error)
	if len(result.MissingMeta) > 0 {
		hasOutput = true
		ui.Error("Missing .meta files (%d):", len(result.MissingMeta))
		for _, path := range result.MissingMeta {
			fmt.Printf("  %s\n", path)
		}
		fmt.Println()
	}

	// Duplicate GUIDs (Error)
	if len(result.DuplicateGUIDs) > 0 {
		hasOutput = true
		ui.Error("Duplicate GUIDs (%d):", len(result.DuplicateGUIDs))
		for guid, files := range result.DuplicateGUIDs {
			fmt.Printf("  GUID: %s\n", guid)
			for _, file := range files {
				fmt.Printf("    - %s\n", file)
			}
		}
		fmt.Println()
	}

	// Orphan meta files (Warning)
	if len(result.OrphanMeta) > 0 {
		hasOutput = true
		ui.Warn("Orphan .meta files (%d):", len(result.OrphanMeta))
		for _, path := range result.OrphanMeta {
			fmt.Printf("  %s\n", path)
		}
		fmt.Println()
	}

	// --fix: generate missing + remove orphans
	if metaCheckFix && (len(result.MissingMeta) > 0 || len(result.OrphanMeta) > 0) {
		if !metaCheckForce {
			actions := []string{}
			if len(result.MissingMeta) > 0 {
				actions = append(actions, fmt.Sprintf("generate %d missing .meta", len(result.MissingMeta)))
			}
			if len(result.OrphanMeta) > 0 {
				actions = append(actions, fmt.Sprintf("remove %d orphan .meta", len(result.OrphanMeta)))
			}
			fmt.Printf("Fix: %s? [y/N]: ", strings.Join(actions, " and "))
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				ui.Muted("Skipped. No changes were made.")
				return exitWithCode(result)
			}
		}

		// Generate missing .meta files
		if len(result.MissingMeta) > 0 {
			genResult, err := checker.Generate(false)
			if err != nil {
				return fmt.Errorf("failed to generate .meta files: %w", err)
			}
			ui.Success("Generated %d .meta file(s)", len(genResult.Generated))
			for _, path := range genResult.Generated {
				fmt.Printf("  %s\n", path)
			}
		}

		// Remove orphan .meta files
		if len(result.OrphanMeta) > 0 {
			deleted, err := checker.Fix(false)
			if err != nil {
				return fmt.Errorf("failed to remove orphan .meta files: %w", err)
			}
			ui.Success("Removed %d orphan .meta file(s)", len(deleted))
		}

		return nil
	}

	if !hasOutput {
		ui.Success("No issues found")
	}

	return exitWithCode(result)
}

func exitWithCode(result *unity.MetaCheckResult) error {
	if result.HasErrors() {
		os.Exit(1)
	}
	return nil
}
