package cmd

import "github.com/spf13/cobra"

var toolCmd = &cobra.Command{
	Use:     "tool",
	Aliases: []string{"tools"},
	Short:   "Interact with Unity tools through the Go daemon",
	Long: `Interact with Unity tools exposed by connected Unity editors.

These commands talk to the Go daemon and the Unity C# bridge over the shared
local JSON-RPC protocol.`,
}

func init() {
	rootCmd.AddCommand(toolCmd)
}
