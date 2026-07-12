package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/neptaco/uniforge/pkg/bridge"
	"gopkg.in/yaml.v3"
)

type toolClientOptions struct {
	project         string
	output          string
	timeoutMS       int
	autoStartDaemon bool
}

func newToolClient(options toolClientOptions) *bridge.Client {
	return bridge.NewClient(bridge.ClientOptions{
		DaemonConfig:    daemonConfig(),
		AutoStartDaemon: options.autoStartDaemon,
		RequestTimeout:  durationFromMillis(options.timeoutMS),
	})
}

func durationFromMillis(value int) time.Duration {
	if value <= 0 {
		return 30 * time.Second
	}
	return time.Duration(value) * time.Millisecond
}

func resolveToolProject(client *bridge.Client, explicitProject string, includeTools bool) (*bridge.ProjectInfo, []bridge.ProjectInfo, error) {
	projectsResult, err := client.ListProjects(includeTools)
	if err != nil {
		return nil, nil, err
	}

	project, err := bridge.ResolveProject(explicitProject, bridge.ResolveFromCwd(""), projectsResult.Projects)
	if err != nil {
		return nil, projectsResult.Projects, err
	}

	return project, projectsResult.Projects, nil
}

func readToolArgs(jsonArg string) (map[string]any, error) {
	if jsonArg != "" {
		return parseJSONObject(jsonArg)
	}

	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		stdin, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		if len(stdin) > 0 {
			return parseJSONObject(string(stdin))
		}
	}

	return map[string]any{}, nil
}

func parseJSONObject(raw string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return map[string]any{}, nil
	}
	return payload, nil
}

func writeStructuredOutput(format string, payload any) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(payload)
	case "yaml":
		if text, ok := payload.(string); ok {
			_, err := fmt.Fprintln(os.Stdout, text)
			return err
		}
		data, err := yaml.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(os.Stdout, string(data))
		return err
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

func writeToolVerbose(enabled bool, format string, args ...any) {
	if !enabled {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
}
