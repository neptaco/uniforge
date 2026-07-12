package cmd

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neptaco/uniforge/pkg/bridge"
	mcppkg "github.com/neptaco/uniforge/pkg/mcp"
	"github.com/spf13/cobra"
)

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start MCP server over stdio",
	RunE:  runMCPServe,
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
}

func runMCPServe(cmd *cobra.Command, args []string) error {
	runtime := mcppkg.NewBridgeRuntime(mcppkg.BridgeRuntimeOptions{
		DaemonConfig:    daemonConfig(),
		AutoStartDaemon: true,
		RequestTimeout:  durationFromMillis(300000),
		CwdHints:        bridge.ResolveFromCwd(""),
	})

	server := mcppkg.NewServer(runtime, mcppkg.ServerOptions{
		Name:    "uniforge",
		Version: Version,
	})

	return server.Run(context.Background(), &mcpsdk.StdioTransport{})
}
