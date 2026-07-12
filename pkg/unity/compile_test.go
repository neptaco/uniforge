package unity

import (
	"errors"
	"fmt"
	"testing"

	"github.com/neptaco/uniforge/pkg/logger"
)

func TestCompileDiagnosticsCollectorCapturesCompileErrorsFromAllLines(t *testing.T) {
	collector := &compileDiagnosticsCollector{}

	for i := 0; i < maxCollectedCompileDiagnostics+25; i++ {
		collector.Observe(fmt.Sprintf("Error: runtime noise %d", i), logger.LogLevelError)
	}

	collector.Observe("Assets/Scripts/Foo.cs(10,5): error CS1002: ; expected", logger.LogLevelError)
	collector.Observe("CompilationHadFailure: True", logger.LogLevelNormal)
	collector.Observe("Scripts have compiler errors", logger.LogLevelNormal)

	if collector.errorCount != 3 {
		t.Fatalf("errorCount = %d, want 3", collector.errorCount)
	}

	if len(collector.errors) != 3 {
		t.Fatalf("len(errors) = %d, want 3", len(collector.errors))
	}
}

func TestCompileDiagnosticsCollectorTracksTruncation(t *testing.T) {
	collector := &compileDiagnosticsCollector{}

	for i := 0; i < maxCollectedCompileDiagnostics+5; i++ {
		collector.Observe(
			fmt.Sprintf("Assets/Scripts/Foo.cs(%d,1): error CS1002: ; expected", i+1),
			logger.LogLevelError,
		)
		collector.Observe(fmt.Sprintf("warning CS0168: unused variable %d", i), logger.LogLevelWarning)
	}

	if collector.errorCount != maxCollectedCompileDiagnostics+5 {
		t.Fatalf("errorCount = %d, want %d", collector.errorCount, maxCollectedCompileDiagnostics+5)
	}
	if collector.warningCount != maxCollectedCompileDiagnostics+5 {
		t.Fatalf("warningCount = %d, want %d", collector.warningCount, maxCollectedCompileDiagnostics+5)
	}
	if len(collector.errors) != maxCollectedCompileDiagnostics {
		t.Fatalf("len(errors) = %d, want %d", len(collector.errors), maxCollectedCompileDiagnostics)
	}
	if len(collector.warnings) != maxCollectedCompileDiagnostics {
		t.Fatalf("len(warnings) = %d, want %d", len(collector.warnings), maxCollectedCompileDiagnostics)
	}
	if !collector.errorsTruncated {
		t.Fatal("errorsTruncated = false, want true")
	}
	if !collector.warningsTruncated {
		t.Fatal("warningsTruncated = false, want true")
	}
}

func TestBuildCompileResultFailsWhenCompileErrorsWereDetected(t *testing.T) {
	collector := &compileDiagnosticsCollector{
		errorCount:   2,
		warningCount: 1,
		errors:       []string{"error A", "error B"},
		warnings:     []string{"warning A"},
	}

	result := buildCompileResult(collector, nil)
	if result.Success {
		t.Fatal("Success = true, want false")
	}
	if result.ErrorCount != 2 {
		t.Fatalf("ErrorCount = %d, want 2", result.ErrorCount)
	}
	if result.WarningCount != 1 {
		t.Fatalf("WarningCount = %d, want 1", result.WarningCount)
	}
}

func TestBuildCompileResultFailsWhenProcessReturnsError(t *testing.T) {
	collector := &compileDiagnosticsCollector{}

	result := buildCompileResult(collector, errors.New("unity exited with failure"))
	if result.Success {
		t.Fatal("Success = true, want false")
	}
}

func TestBuildCompileResultSucceedsWithoutErrors(t *testing.T) {
	collector := &compileDiagnosticsCollector{warningCount: 3}

	result := buildCompileResult(collector, nil)
	if !result.Success {
		t.Fatal("Success = false, want true")
	}
	if result.WarningCount != 3 {
		t.Fatalf("WarningCount = %d, want 3", result.WarningCount)
	}
}
