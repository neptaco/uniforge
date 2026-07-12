package cmd

import "github.com/spf13/cobra"

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP integration commands",
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
