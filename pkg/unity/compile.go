package unity

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/neptaco/uniforge/pkg/logger"
	"github.com/neptaco/uniforge/pkg/ui"
)

// compileErrorPattern matches C# compiler errors and Unity script errors
var compileErrorPattern = regexp.MustCompile(`(?i)(?:^error CS\d+|^Assets/.*\.cs\(\d+,\d+\):\s*error|compilationhadfailure:\s*True|Scripts have compiler errors)`)

const maxCollectedCompileDiagnostics = 1000

// CompileConfig holds configuration for compile-only execution
type CompileConfig struct {
	ProjectPath    string
	LogFile        string
	TimeoutSeconds int
	CIMode         bool
	ShowTimestamp  bool
}

// CompileResult holds the result of a compile operation
type CompileResult struct {
	Success           bool     `json:"success"`
	ErrorCount        int      `json:"errorCount"`
	WarningCount      int      `json:"warningCount"`
	Errors            []string `json:"errors,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
	ErrorsTruncated   bool     `json:"errorsTruncated,omitempty"`
	WarningsTruncated bool     `json:"warningsTruncated,omitempty"`
}

// Compiler handles Unity compile-only execution
type Compiler struct {
	project *Project
	editor  *Editor
}

type compileDiagnosticsCollector struct {
	errorCount        int
	warningCount      int
	errors            []string
	warnings          []string
	errorsTruncated   bool
	warningsTruncated bool
}

func (c *compileDiagnosticsCollector) Observe(line string, level logger.LogLevel) {
	trimmed := strings.TrimSpace(line)

	if compileErrorPattern.MatchString(trimmed) {
		c.errorCount++
		if len(c.errors) < maxCollectedCompileDiagnostics {
			c.errors = append(c.errors, line)
		} else {
			c.errorsTruncated = true
		}
		return
	}

	if level == logger.LogLevelWarning {
		c.warningCount++
		if len(c.warnings) < maxCollectedCompileDiagnostics {
			c.warnings = append(c.warnings, line)
		} else {
			c.warningsTruncated = true
		}
	}
}

// NewCompiler creates a new Compiler
func NewCompiler(project *Project) *Compiler {
	return &Compiler{
		project: project,
		editor:  NewEditor(project.UnityVersion),
	}
}

// Compile runs Unity in batch mode to trigger compilation only
func (c *Compiler) Compile(config CompileConfig) (*CompileResult, error) {
	absProjectPath, err := filepath.Abs(config.ProjectPath)
	if err != nil {
		absProjectPath = config.ProjectPath
	}

	// Check if Unity Editor is already running for this project
	if err := c.editor.CheckNotRunning(absProjectPath); err != nil {
		return nil, err
	}

	editorPath, err := c.editor.GetPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get Unity Editor path: %w", err)
	}

	projectName := filepath.Base(absProjectPath)
	args := []string{
		"-projectPath", projectName,
		"-batchmode",
		"-nographics",
		"-quit",
	}

	if config.LogFile != "" {
		args = append(args, "-logFile", config.LogFile)
	} else {
		args = append(args, "-logFile", "-")
	}

	timeout := config.TimeoutSeconds
	if timeout == 0 {
		timeout = 300 // Default 5 minutes for compile
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := newUnityBatchCommand(ctx, editorPath, args...)

	diagnostics := &compileDiagnosticsCollector{}
	log := logger.NewWithOptions(config.LogFile,
		logger.WithCIMode(config.CIMode),
		logger.WithShowTime(config.ShowTimestamp),
		logger.WithLineObserver(diagnostics.Observe),
	)
	defer func() { _ = log.Close() }()

	cmd.Stdout = log
	cmd.Stderr = log

	projectDir := filepath.Dir(absProjectPath)
	cmd.Dir = projectDir

	ui.Debug("Running Unity compile", "path", editorPath, "args", strings.Join(args, " "))

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Unity: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("compile timeout after %d seconds", timeout)
		}
		return buildCompileResult(diagnostics, err), nil
	}

	return buildCompileResult(diagnostics, nil), nil
}

func buildCompileResult(diagnostics *compileDiagnosticsCollector, waitErr error) *CompileResult {
	result := &CompileResult{
		Success:           waitErr == nil && diagnostics.errorCount == 0,
		ErrorCount:        diagnostics.errorCount,
		WarningCount:      diagnostics.warningCount,
		Errors:            diagnostics.errors,
		Warnings:          diagnostics.warnings,
		ErrorsTruncated:   diagnostics.errorsTruncated,
		WarningsTruncated: diagnostics.warningsTruncated,
	}

	return result
}
