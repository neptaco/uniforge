package unity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMetaGenerator_GeneratesMetaForSupportedAsset(t *testing.T) {
	project, tempDir := setupTestProject(t)
	assetsDir := filepath.Join(tempDir, "Assets")

	createAssetWithoutMeta(t, assetsDir, "Script.cs")

	generator := NewMetaGenerator(project)
	result, err := generator.Generate(MetaGenerateOptions{})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(result.Generated) != 1 {
		t.Fatalf("expected 1 generated meta file, got %d", len(result.Generated))
	}

	if result.Generated[0] != "Assets/Script.cs" {
		t.Fatalf("unexpected generated path: %v", result.Generated[0])
	}

	if len(result.Skipped) != 0 {
		t.Fatalf("expected no skipped assets, got %d", len(result.Skipped))
	}

	metaPath := filepath.Join(assetsDir, "Script.cs.meta")
	if _, err := os.Stat(metaPath); err != nil {
		t.Fatalf("meta file was not created: %v", err)
	}
}

func TestMetaGenerator_DryRunDoesNotWriteFiles(t *testing.T) {
	project, tempDir := setupTestProject(t)
	assetsDir := filepath.Join(tempDir, "Assets")

	createAssetWithoutMeta(t, assetsDir, "DryRun.cs")

	generator := NewMetaGenerator(project)
	result, err := generator.Generate(MetaGenerateOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(result.Generated) != 1 {
		t.Fatalf("expected 1 generated path in dry run, got %d", len(result.Generated))
	}

	if len(result.Skipped) != 0 {
		t.Fatalf("expected no skipped assets in dry run, got %d", len(result.Skipped))
	}

	metaPath := filepath.Join(assetsDir, "DryRun.cs.meta")
	if _, err := os.Stat(metaPath); err == nil {
		t.Fatalf("meta file should not be created during dry run")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error: %v", err)
	}
}

func TestMetaGenerator_SkipsUnsupportedAssetTypes(t *testing.T) {
	project, tempDir := setupTestProject(t)
	assetsDir := filepath.Join(tempDir, "Assets")

	createAssetWithoutMeta(t, assetsDir, "Native.dll")

	generator := NewMetaGenerator(project)
	result, err := generator.Generate(MetaGenerateOptions{})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(result.Generated) != 0 {
		t.Fatalf("expected no generated meta files, got %d", len(result.Generated))
	}

	if len(result.Skipped) != 1 {
		t.Fatalf("expected one skipped asset, got %d", len(result.Skipped))
	}

	if result.Skipped[0].Path != "Assets/Native.dll" {
		t.Fatalf("unexpected skipped path: %v", result.Skipped[0].Path)
	}

	if _, err := os.Stat(filepath.Join(assetsDir, "Native.dll.meta")); err == nil {
		t.Fatalf("unsupported asset should not get a meta file")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error: %v", err)
	}
}

func TestMetaGenerator_TargetPathFiltersAssets(t *testing.T) {
	project, tempDir := setupTestProject(t)
	assetsDir := filepath.Join(tempDir, "Assets")

	createAssetWithoutMeta(t, assetsDir, "First.cs")
	createAssetWithoutMeta(t, assetsDir, "Second.cs")

	generator := NewMetaGenerator(project)
	result, err := generator.Generate(MetaGenerateOptions{TargetPath: "Assets/First.cs"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(result.Generated) != 1 {
		t.Fatalf("expected one generated meta file, got %d", len(result.Generated))
	}

	if result.Generated[0] != "Assets/First.cs" {
		t.Fatalf("unexpected generated entry: %v", result.Generated[0])
	}

	if _, err := os.Stat(filepath.Join(assetsDir, "First.cs.meta")); err != nil {
		t.Fatalf("expected meta for first asset: %v", err)
	}

	if _, err := os.Stat(filepath.Join(assetsDir, "Second.cs.meta")); err == nil {
		t.Fatalf("second asset should remain without meta")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error: %v", err)
	}
}
