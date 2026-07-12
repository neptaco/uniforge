package unity

import "testing"

func TestFindUnityProcessFromWindowsJSONMatchesProjectPath(t *testing.T) {
	output := []byte(`[
  {"ProcessId":123,"CommandLine":"C:\\Program Files\\Unity\\Editor\\Unity.exe -projectPath C:\\Projects\\Other"},
  {"ProcessId":456,"CommandLine":"\"C:\\Program Files\\Unity\\Editor\\Unity.exe\" -projectPath \"C:\\Projects\\My Game\""}
]`)

	pid, err := findUnityProcessFromWindowsJSON(output, `C:\Projects\My Game`)
	if err != nil {
		t.Fatalf("findUnityProcessFromWindowsJSON failed: %v", err)
	}
	if pid != 456 {
		t.Fatalf("pid = %d, want 456", pid)
	}
}

func TestFindUnityProcessFromWindowsJSONMatchesCaseAndSlashInsensitively(t *testing.T) {
	output := []byte(`[{"ProcessId":456,"CommandLine":"Unity.exe -projectPath C:\\PROJECTS\\MY GAME"}]`)

	pid, err := findUnityProcessFromWindowsJSON(output, `c:/projects/my game`)
	if err != nil {
		t.Fatalf("findUnityProcessFromWindowsJSON failed: %v", err)
	}
	if pid != 456 {
		t.Fatalf("pid = %d, want 456", pid)
	}
}

func TestFindUnityProcessFromWindowsJSONHandlesEmptyProcessList(t *testing.T) {
	pid, err := findUnityProcessFromWindowsJSON([]byte(`[]`), `C:\Projects\MyGame`)
	if err != nil {
		t.Fatalf("findUnityProcessFromWindowsJSON failed: %v", err)
	}
	if pid != 0 {
		t.Fatalf("pid = %d, want 0", pid)
	}
}

func TestFindUnityProcessFromWindowsJSONIgnoresMissingCommandLine(t *testing.T) {
	output := []byte(`[{"ProcessId":123,"CommandLine":null}]`)

	pid, err := findUnityProcessFromWindowsJSON(output, `C:\Projects\MyGame`)
	if err != nil {
		t.Fatalf("findUnityProcessFromWindowsJSON failed: %v", err)
	}
	if pid != 0 {
		t.Fatalf("pid = %d, want 0", pid)
	}
}

func TestFindUnityProcessFromWindowsJSONRejectsInvalidJSON(t *testing.T) {
	if _, err := findUnityProcessFromWindowsJSON([]byte(`not-json`), `C:\Projects\MyGame`); err == nil {
		t.Fatal("expected invalid JSON to return an error")
	}
}

func TestFindUnityProcessFromWindowsJSONRejectsEmptyProjectPath(t *testing.T) {
	if _, err := findUnityProcessFromWindowsJSON([]byte(`[]`), ""); err == nil {
		t.Fatal("expected an empty project path to return an error")
	}
}
