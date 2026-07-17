package cmd

import "github.com/spf13/cobra"

var batchCmd = &cobra.Command{
	Use:        "batch",
	Short:      "Deprecated Unity batch mode command group",
	Deprecated: "use the root-level `compile`, `test`, and `run` commands instead",
	Hidden:     true,
}

func init() {
	rootCmd.AddCommand(batchCmd)
}
