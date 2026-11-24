package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neptaco/uniforge/pkg/bridge"
)

func TestSelectEvalSource(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		file, piped bool
		want        evalSource
		wantError   bool
	}{
		{name: "argument", args: []string{"2 + 2"}, want: evalArgument},
		{name: "file", file: true, want: evalFile},
		{name: "dash", args: []string{"-"}, want: evalStdin},
		{name: "pipe", piped: true, want: evalStdin},
		{name: "explicit argument with redirected stdin", args: []string{"2 + 2"}, piped: true, want: evalArgument},
		{name: "explicit file with redirected stdin", file: true, piped: true, want: evalFile},
		{name: "argument and file", args: []string{"2 + 2"}, file: true, wantError: true},
		{name: "dash and file", args: []string{"-"}, file: true, wantError: true},
		{name: "multiple arguments", args: []string{"2", "2"}, wantError: true},
		{name: "no source", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectEvalSource(tt.args, tt.file, tt.piped)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected an input source error")
				}
				return
			}
			if got != tt.want || err != nil {
				t.Fatalf("source = %v, error = %v, want %v", got, err, tt.want)
			}
		})
	}
}

func TestReadEvalCode(t *testing.T) {
	tests := []struct {
		name   string
		source evalSource
		input  string
	}{
		{name: "argument", source: evalArgument, input: "  2 + 2  "},
		{name: "file", source: evalFile, input: "\nreturn 4;\n"},
		{name: "stdin", source: evalStdin, input: "\nreturn 4;\n"},
		{name: "empty argument", source: evalArgument},
		{name: "empty file", source: evalFile},
		{name: "whitespace stdin", source: evalStdin, input: " \n\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var file string
			if tt.source == evalFile {
				file = filepath.Join(t.TempDir(), "script.cs")
				if err := os.WriteFile(file, []byte(tt.input), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := readEvalCode(tt.source, []string{tt.input}, file, strings.NewReader(tt.input))
			if strings.TrimSpace(tt.input) == "" {
				if err == nil || err.Error() != "no C# code provided" {
					t.Fatalf("error = %v, want empty code error", err)
				}
				return
			}
			if got != tt.input || err != nil {
				t.Fatalf("code = %q, error = %v, want %q", got, err, tt.input)
			}
		})
	}
}

func TestValidateEvalTimeout(t *testing.T) {
	for _, seconds := range []int{-1, 0, 1, 30, 600, 601} {
		wantError := seconds < 1 || seconds > 600
		if err := validateEvalTimeout(seconds); (err != nil) != wantError {
			t.Errorf("timeout = %d: error = %v, want error = %v", seconds, err, wantError)
		}
	}
}

func TestFormatEvalTextSuccess(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		dryRun bool
		want   string
	}{
		{name: "string", value: "Hello\nWorld", want: "Hello\nWorld"},
		{name: "empty string", value: "", want: ""},
		{name: "number", value: 4.25, want: "4.25"},
		{name: "bool", value: false, want: "false"},
		{name: "null", want: "null"},
		{name: "array", value: []any{1, "two"}, want: "[\n  1,\n  \"two\"\n]"},
		{name: "object", value: map[string]any{"ok": true}, want: "{\n  \"ok\": true\n}"},
		{name: "dry run", dryRun: true, want: "Compilation succeeded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatEvalOutput("text", tt.dryRun, true, map[string]any{"value": tt.value}, nil)
			if got != tt.want || err != nil {
				t.Fatalf("output = %q, error = %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestFormatEvalTextFailure(t *testing.T) {
	tests := []struct {
		name, payload, wantError string
	}{
		{
			name: "compilation error",
			payload: `{"error_code":"COMPILATION_ERROR","diagnostics":[
				{"severity":"error","line":2,"column":8,"message":"Expected ;"},
				{"severity":"warning","line":0,"column":1,"message":"Unreachable code"}
			]}`,
			wantError: "eval(2,8): error Expected ;\neval(0,1): warning Unreachable code",
		},
		{
			name: "runtime exception",
			payload: `{"error_code":"RUNTIME_EXCEPTION","exception":{
				"type":"System.InvalidOperationException","message":"Invalid state","stack_trace":"at Eval.Run()"
			}}`,
			wantError: "System.InvalidOperationException: Invalid state\nat Eval.Run()",
		},
		{name: "busy", payload: `{"error_code":"BUSY","message":"Unity is compiling"}`, wantError: "error: Unity is compiling"},
		{name: "disabled", payload: `{"error_code":"DISABLED","message":"Eval is disabled"}`, wantError: "error: Eval is disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal([]byte(tt.payload), &payload); err != nil {
				t.Fatal(err)
			}
			got, err := formatEvalOutput("text", false, false, payload, nil)
			if got != "" || err == nil || err.Error() != tt.wantError {
				t.Fatalf("output = %q, error = %v, want error %q", got, err, tt.wantError)
			}
		})
	}
}

func TestFormatEvalJSONPreservesToolPayload(t *testing.T) {
	payload := map[string]any{
		"success": false, "error_code": "BUSY", "message": "Unity is busy", "compile_ms": 0,
	}
	got, err := formatEvalOutput("json", false, false, payload, nil)
	if err == nil || err.Error() != "Unity is busy" {
		t.Fatalf("error = %v, want Unity is busy", err)
	}
	want := "{\n  \"compile_ms\": 0,\n  \"error_code\": \"BUSY\",\n  \"message\": \"Unity is busy\",\n  \"success\": false\n}"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestEvalCommandWritesJSONForFailureWithoutPayload(t *testing.T) {
	command := newEvalCommand()
	command.SetArgs([]string{"2 + 2", "-o", "json", "--timeout", "601"})
	var stdout, stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SilenceErrors = true
	command.SilenceUsage = true

	err := command.Execute()
	const message = "--timeout must be between 1 and 600 seconds"
	if err == nil || err.Error() != message {
		t.Fatalf("error = %v, want %q", err, message)
	}
	const want = "{\n  \"error_code\": \"TOOL_ERROR\",\n  \"message\": \"--timeout must be between 1 and 600 seconds\",\n  \"success\": false\n}\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q, want stdout %q and empty stderr before root handles error", &stdout, &stderr, want)
	}
}

func TestValidateEvalProject(t *testing.T) {
	root := t.TempDir()
	cwdProject := filepath.Join(root, "Game")
	other := bridge.ProjectInfo{ID: filepath.Join(root, "Other"), GitRoot: root}
	for _, dir := range []string{"Assets", "ProjectSettings"} {
		if err := os.MkdirAll(filepath.Join(cwdProject, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	hints := bridge.ResolveFromCwd(filepath.Join(cwdProject, "Assets"))
	if err := validateEvalProject("", hints.ProjectPath, other); err == nil || !strings.Contains(err.Error(), "--project") {
		t.Fatalf("error = %v, want mismatch guidance", err)
	}
	if err := validateEvalProject("Other", hints.ProjectPath, other); err != nil {
		t.Fatalf("explicit project returned an error: %v", err)
	}
	if err := validateEvalProject("", "", other); err != nil {
		t.Fatalf("cwd outside a Unity project returned an error: %v", err)
	}
	if err := validateEvalProject("", hints.ProjectPath, bridge.ProjectInfo{ID: cwdProject}); err != nil {
		t.Fatalf("matching project returned an error: %v", err)
	}
}

func TestEvalToolNotFoundGuidance(t *testing.T) {
	missing := &bridge.DaemonError{Code: bridge.ToolNotFound, Message: "Tool not found: eval"}
	output, err := formatEvalOutput("text", false, false, nil, evalToolError(missing))
	if output != "" || !errors.Is(err, missing) {
		t.Fatalf("output = %q, error = %v, want wrapped daemon error", output, err)
	}
	if !strings.HasPrefix(err.Error(), "error: Tool not found: eval") ||
		!strings.Contains(err.Error(), "uniforge package update") || !strings.Contains(err.Error(), "ToolSettings") {
		t.Fatalf("missing tool guidance: %v", err)
	}
	other := &bridge.DaemonError{Code: bridge.ToolTimeout, Message: "tool execution timed out"}
	if got := evalToolError(other); got != other {
		t.Fatalf("unexpected guidance for timeout: %v", got)
	}
}
