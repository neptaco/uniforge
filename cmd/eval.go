package cmd

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/spf13/cobra"
)

type evalOptions struct {
	file, project, output string
	timeout               int
	dryRun, autoStart     bool
}

var evalCmd = newEvalCommand()

func init() {
	rootCmd.AddCommand(evalCmd)
}

func newEvalCommand() *cobra.Command {
	var options evalOptions
	command := &cobra.Command{
		Use:   "eval [code]",
		Short: "Evaluate C# in a running Unity Editor",
		Long: `Compile and run C# in a connected Unity Editor and print the returned value.

The code is the body of an async method: use statements with "return", or a
single expression. Leading "using" lines, local functions, "await", and the
"cancellationToken" variable are available. Code can modify the Editor and its
assets. A synchronous infinite loop cannot be stopped by --timeout.

Examples:
  # Evaluate an expression
  uniforge eval 'Application.unityVersion'

  # Run statements
  uniforge eval 'var go = new GameObject("Probe"); return go.name;'

  # Run a file, or compile it without running
  uniforge eval --file script.cs
  uniforge eval --file script.cs --dry-run

  # Read from standard input and print the full result as JSON
  cat script.cs | uniforge eval - -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEval(cmd, args, options)
		},
	}
	flags := command.Flags()
	flags.StringVarP(&options.file, "file", "f", "", "Read C# code from a file")
	flags.StringVarP(&options.project, "project", "p", "", "Project ID or name")
	flags.IntVar(&options.timeout, "timeout", 30, "Timeout in seconds for each of compilation and execution (1–600)")
	flags.BoolVar(&options.dryRun, "dry-run", false, "Compile without executing")
	flags.StringVarP(&options.output, "output", "o", "text", "Output format: text, json")
	flags.BoolVar(&options.autoStart, "auto-start-daemon", true, "Start the daemon automatically if needed")
	return command
}

func runEval(cmd *cobra.Command, args []string, options evalOptions) error {
	if options.output != "text" && options.output != "json" {
		return fmt.Errorf("error: unsupported output format: %s (use text or json)", options.output)
	}
	result, err := evaluate(cmd, args, options)
	if err != nil {
		err = evalToolError(err)
	}
	output, resultErr := formatEvalOutput(options.output, options.dryRun, result.Success, result.Result, err)
	if output != "" || resultErr == nil {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), output); err != nil {
			return fmt.Errorf("write eval output: %w", err)
		}
	}
	return resultErr
}

func evaluate(cmd *cobra.Command, args []string, options evalOptions) (bridge.ClientToolCallResult, error) {
	var empty bridge.ClientToolCallResult
	if err := validateEvalTimeout(options.timeout); err != nil {
		return empty, err
	}
	stdin := cmd.InOrStdin()
	source, err := selectEvalSource(args, cmd.Flags().Changed("file"), evalStdinRedirected(stdin))
	if err != nil {
		return empty, err
	}
	code, err := readEvalCode(source, args, options.file, stdin)
	if err != nil {
		return empty, err
	}

	requestTimeoutMS := (options.timeout*2 + 10) * 1000
	client := newToolClient(toolClientOptions{
		timeoutMS:       requestTimeoutMS,
		autoStartDaemon: options.autoStart,
	})
	defer func() { _ = client.Close() }()
	if err := client.Connect(); err != nil {
		return empty, err
	}
	if _, err := client.Register(); err != nil {
		return empty, err
	}
	project, _, err := resolveToolProject(client, options.project, false)
	if err != nil {
		return empty, err
	}
	if err := validateEvalProject(options.project, bridge.ResolveFromCwd("").ProjectPath, *project); err != nil {
		return empty, err
	}

	result, err := client.ToolCall("eval", map[string]any{
		"code":       code,
		"timeout_ms": options.timeout * 1000,
		"dry_run":    options.dryRun,
		"request_id": rand.Text(),
	}, project.ID, durationFromMillis(requestTimeoutMS))
	if err != nil {
		return empty, err
	}
	if result.Result == nil {
		message := result.Error
		if message == "" {
			message = "eval returned no result"
		}
		return empty, errors.New(message)
	}
	return *result, nil
}

func validateEvalTimeout(seconds int) error {
	if seconds < 1 || seconds > 600 {
		return errors.New("--timeout must be between 1 and 600 seconds")
	}
	return nil
}

func validateEvalProject(explicit, cwdProject string, project bridge.ProjectInfo) error {
	if explicit == "" && cwdProject != "" && bridge.MatchProject(bridge.CwdHints{ProjectPath: cwdProject}, []bridge.ProjectInfo{project}) == nil {
		return fmt.Errorf("current Unity project %q differs from connected project %q; use --project to select the intended project", cwdProject, project.ID)
	}
	return nil
}

func evalToolError(err error) error {
	var daemonErr *bridge.DaemonError
	if errors.As(err, &daemonErr) && daemonErr.Code == bridge.ToolNotFound {
		return fmt.Errorf("%w; the Unity package may be out of date (run `uniforge package update`), or eval may be disabled in ToolSettings", err)
	}
	return err
}

type evalSource int

const (
	evalArgument evalSource = iota
	evalFile
	evalStdin
)

func selectEvalSource(args []string, fileSpecified, stdinRedirected bool) (evalSource, error) {
	if len(args) > 1 || (len(args) == 1 && fileSpecified) {
		return 0, errors.New("provide C# code from only one source: an argument, --file, or standard input")
	}
	switch {
	case fileSpecified:
		return evalFile, nil
	case len(args) == 1 && args[0] == "-":
		return evalStdin, nil
	case len(args) == 1:
		return evalArgument, nil
	case stdinRedirected:
		return evalStdin, nil
	default:
		return 0, errors.New("provide C# code as an argument, with --file, or on standard input")
	}
}

func evalStdinRedirected(stdin io.Reader) bool {
	file, ok := stdin.(*os.File)
	if !ok {
		return true
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice == 0
}

func readEvalCode(source evalSource, args []string, file string, stdin io.Reader) (string, error) {
	var code string
	switch source {
	case evalArgument:
		code = args[0]
	case evalFile:
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read C# file: %w", err)
		}
		code = string(data)
	case evalStdin:
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read C# from standard input: %w", err)
		}
		code = string(data)
	}
	if strings.TrimSpace(code) == "" {
		return "", errors.New("no C# code provided")
	}
	return code, nil
}

type evalPayload struct {
	Value       any
	ErrorCode   string `json:"error_code"`
	Message     string
	Diagnostics []struct {
		Severity     string
		Line, Column int
		Message      string
	}
	Exception *struct {
		Type, Message string
		StackTrace    string `json:"stack_trace"`
	}
}

func formatEvalOutput(format string, dryRun, success bool, payload any, toolErr error) (string, error) {
	if toolErr != nil {
		if format == "text" {
			return "", fmt.Errorf("error: %w", toolErr)
		}
		payload = map[string]any{"success": false, "error_code": "TOOL_ERROR", "message": toolErr.Error()}
	}
	if format == "json" {
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return "", err
		}
		if toolErr == nil && !success {
			fields, _ := payload.(map[string]any)
			message, _ := fields["message"].(string)
			toolErr = errors.New(evalFailureMessage(message))
		}
		return string(data), toolErr
	}

	if success && dryRun {
		return "Compilation succeeded", nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var result evalPayload
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if !success {
		return "", formatEvalFailure(result)
	}
	switch value := result.Value.(type) {
	case string:
		return value, nil
	case []any, map[string]any:
		data, err := json.MarshalIndent(value, "", "  ")
		return string(data), err
	default:
		data, err := json.Marshal(value)
		return string(data), err
	}
}

func formatEvalFailure(result evalPayload) error {
	switch result.ErrorCode {
	case "COMPILATION_ERROR":
		if len(result.Diagnostics) > 0 {
			lines := make([]string, 0, len(result.Diagnostics))
			for _, diagnostic := range result.Diagnostics {
				lines = append(lines, fmt.Sprintf("eval(%d,%d): %s %s", diagnostic.Line, diagnostic.Column, diagnostic.Severity, diagnostic.Message))
			}
			return errors.New(strings.Join(lines, "\n"))
		}
	case "RUNTIME_EXCEPTION":
		if exception := result.Exception; exception != nil {
			message := fmt.Sprintf("%s: %s", exception.Type, exception.Message)
			if exception.StackTrace != "" {
				message += "\n" + exception.StackTrace
			}
			return errors.New(message)
		}
	}
	return fmt.Errorf("error: %s", evalFailureMessage(result.Message))
}

func evalFailureMessage(message string) string {
	if message == "" {
		return "eval failed"
	}
	return message
}
