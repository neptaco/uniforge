package unity

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildArgs_Basic(t *testing.T) {
	runner := &TestRunner{
		project: &Project{UnityVersion: "2022.3.10f1"},
		editor:  NewEditor("2022.3.10f1"),
	}

	config := TestConfig{
		Platform: TestPlatformEditMode,
	}

	args, resultsFile := runner.buildArgs("/path/to/MyProject", config)

	if resultsFile != "" {
		t.Errorf("expected empty resultsFile, got %q", resultsFile)
	}

	assertContains(t, args, "-projectPath", "MyProject")
	assertContains(t, args, "-batchmode", "")
	assertContains(t, args, "-testPlatform", "editmode")
	assertContains(t, args, "-logFile", "-")
}

func TestBuildArgs_WithResultsFile(t *testing.T) {
	runner := &TestRunner{
		project: &Project{UnityVersion: "2022.3.10f1"},
		editor:  NewEditor("2022.3.10f1"),
	}

	config := TestConfig{
		Platform:    TestPlatformEditMode,
		ResultsFile: "/tmp/results.xml",
	}

	args, resultsFile := runner.buildArgs("/path/to/MyProject", config)

	if resultsFile != "/tmp/results.xml" {
		t.Errorf("expected resultsFile /tmp/results.xml, got %q", resultsFile)
	}
	assertContains(t, args, "-testResults", "/tmp/results.xml")
}

func TestBuildArgs_WithResultsDir(t *testing.T) {
	runner := &TestRunner{
		project: &Project{UnityVersion: "2022.3.10f1"},
		editor:  NewEditor("2022.3.10f1"),
	}

	config := TestConfig{
		Platform:   TestPlatformEditMode,
		ResultsDir: "/tmp/test-results",
	}

	args, resultsFile := runner.buildArgs("/path/to/MyProject", config)

	expected := filepath.Join("/tmp/test-results", "TestResults-editmode.xml")
	if resultsFile != expected {
		t.Errorf("expected resultsFile %q, got %q", expected, resultsFile)
	}
	assertContains(t, args, "-testResults", expected)
}

func TestBuildArgs_ResultsFileTakesPrecedenceOverResultsDir(t *testing.T) {
	runner := &TestRunner{
		project: &Project{UnityVersion: "2022.3.10f1"},
		editor:  NewEditor("2022.3.10f1"),
	}

	config := TestConfig{
		Platform:    TestPlatformEditMode,
		ResultsFile: "/explicit/path.xml",
		ResultsDir:  "/tmp/test-results",
	}

	args, resultsFile := runner.buildArgs("/path/to/MyProject", config)

	if resultsFile != "/explicit/path.xml" {
		t.Errorf("expected resultsFile /explicit/path.xml, got %q", resultsFile)
	}
	assertContains(t, args, "-testResults", "/explicit/path.xml")
}

func TestBuildArgs_WithFilter(t *testing.T) {
	runner := &TestRunner{
		project: &Project{UnityVersion: "2022.3.10f1"},
		editor:  NewEditor("2022.3.10f1"),
	}

	config := TestConfig{
		Platform: TestPlatformEditMode,
		Filter:   "MyTestClass",
	}

	args, _ := runner.buildArgs("/path/to/MyProject", config)
	assertContains(t, args, "-testFilter", "MyTestClass")
}

func TestParseTestResults_ValidXML(t *testing.T) {
	xml := `<?xml version="1.0" encoding="utf-8"?>
<test-run id="1" total="328" passed="325" failed="2" skipped="1" duration="0.1234">
</test-run>`

	tmpFile := filepath.Join(t.TempDir(), "results.xml")
	if err := os.WriteFile(tmpFile, []byte(xml), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := parseTestResults(tmpFile)
	if err != nil {
		t.Fatalf("parseTestResults failed: %v", err)
	}

	if summary.Total != 328 {
		t.Errorf("Total: expected 328, got %d", summary.Total)
	}
	if summary.Passed != 325 {
		t.Errorf("Passed: expected 325, got %d", summary.Passed)
	}
	if summary.Failed != 2 {
		t.Errorf("Failed: expected 2, got %d", summary.Failed)
	}
	if summary.Skipped != 1 {
		t.Errorf("Skipped: expected 1, got %d", summary.Skipped)
	}
	if summary.Duration < 0.12 || summary.Duration > 0.13 {
		t.Errorf("Duration: expected ~0.1234, got %f", summary.Duration)
	}
}

func TestParseTestResults_EmptyPath(t *testing.T) {
	summary, err := parseTestResults("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if summary != nil {
		t.Errorf("expected nil summary, got %v", summary)
	}
}

func TestParseTestResults_NonExistentFile(t *testing.T) {
	_, err := parseTestResults("/nonexistent/path.xml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestParseTestResults_InvalidXML(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "results.xml")
	if err := os.WriteFile(tmpFile, []byte("not xml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := parseTestResults(tmpFile)
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestEvaluateTestOutcomeFailsWhenResultsContainFailures(t *testing.T) {
	resultsFile := writeTestResults(t, `<?xml version="1.0" encoding="utf-8"?>
<test-run total="3" passed="2" failed="1" skipped="0" duration="0.5"></test-run>`)

	summary, err := evaluateTestOutcome(resultsFile, nil)
	if err == nil {
		t.Fatal("expected failed test summary to return an error")
	}
	if summary == nil || summary.Failed != 1 {
		t.Fatalf("summary = %#v, want one failed test", summary)
	}
}

func TestEvaluateTestOutcomeFailsWhenRequestedResultsAreMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.xml")
	if _, err := evaluateTestOutcome(missing, nil); err == nil {
		t.Fatal("expected missing results file to return an error")
	}
}

func TestEvaluateTestOutcomePreservesSummaryOnProcessFailure(t *testing.T) {
	resultsFile := writeTestResults(t, `<?xml version="1.0" encoding="utf-8"?>
<test-run total="2" passed="1" failed="1" skipped="0" duration="0.5"></test-run>`)
	waitErr := errors.New("Unity exited with code 2")

	summary, err := evaluateTestOutcome(resultsFile, waitErr)
	if !errors.Is(err, waitErr) {
		t.Fatalf("error = %v, want wrapped process error", err)
	}
	if summary == nil || summary.Failed != 1 {
		t.Fatalf("summary = %#v, want one failed test", summary)
	}
}

func writeTestResults(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "results.xml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// assertContains checks that args contains the expected flag and optional value
func assertContains(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i, arg := range args {
		if arg == flag {
			if value == "" {
				return
			}
			if i+1 < len(args) && args[i+1] == value {
				return
			}
			t.Errorf("flag %q found but value %q != %q", flag, args[i+1], value)
			return
		}
	}
	t.Errorf("flag %q not found in args: %v", flag, args)
}
