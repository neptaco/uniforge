package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var (
	compileJSON      bool
	compileLogFile   string
	compileTimeout   int
	compileCIMode    bool
	compileTimestamp bool
)

var compileCmd = &cobra.Command{
	Use:   "compile [project]",
	Short: "Compile Unity project scripts",
	Long: `Compile Unity project scripts without running tests.

Opens the project in batch mode to trigger script compilation,
then exits. Use the exit code to determine success or failure.

Examples:
  # Compile current project
  uniforge compile

  # Compile specific project
  uniforge compile /path/to/project

  # JSON output (for programmatic use)
  uniforge compile --json

  # With timeout
  uniforge compile --timeout 600`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCompile,
}

func init() {
	rootCmd.AddCommand(compileCmd)

	compileCmd.Flags().BoolVar(&compileJSON, "json", false, "Output result in JSON format")
	compileCmd.Flags().StringVar(&compileLogFile, "log-file", "", "Path to save log file")
	compileCmd.Flags().IntVar(&compileTimeout, "timeout", 300, "Compile timeout in seconds")
	compileCmd.Flags().BoolVar(&compileCIMode, "ci", false, "CI mode (optimized output format)")
	compileCmd.Flags().BoolVarP(&compileTimestamp, "timestamp", "t", false, "Show timestamp for each line")
}

func runCompile(cmd *cobra.Command, args []string) error {
	projectPath := "."
	if len(args) > 0 {
		projectPath = args[0]
	}

	if !compileJSON {
		ui.Info("Compiling project: %s", projectPath)
	}

	project, err := unity.LoadProject(projectPath)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	config := unity.CompileConfig{
		ProjectPath:    projectPath,
		LogFile:        compileLogFile,
		TimeoutSeconds: compileTimeout,
		CIMode:         compileCIMode,
		ShowTimestamp:  compileTimestamp,
	}

	compiler := unity.NewCompiler(project)
	result, err := compiler.Compile(config)
	if err != nil {
		return fmt.Errorf("compile failed: %w", err)
	}

	if compileJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(result)
		if !result.Success {
			os.Exit(1)
		}
		return nil
	}

	if result.Success {
		ui.Success("Compilation succeeded")
	} else {
		if result.ErrorCount > 0 {
			if result.ErrorsTruncated {
				ui.Error("Compilation failed with %d error(s) (showing first %d)", result.ErrorCount, len(result.Errors))
			} else {
				ui.Error("Compilation failed with %d error(s)", result.ErrorCount)
			}
		} else {
			ui.Error("Compilation failed")
		}
		return fmt.Errorf("compilation failed")
	}

	return nil
}
