package unity

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MetaCheckResult holds the result of meta file checking.
type MetaCheckResult struct {
	MissingMeta    []string            // Assets without .meta files
	OrphanMeta     []string            // .meta files without corresponding assets
	DuplicateGUIDs map[string][]string // GUID -> list of files with that GUID
}

// HasErrors returns true if there are any errors (missing meta or duplicate GUIDs).
func (r *MetaCheckResult) HasErrors() bool {
	return len(r.MissingMeta) > 0 || len(r.DuplicateGUIDs) > 0
}

// HasWarnings returns true if there are any warnings (orphan meta).
func (r *MetaCheckResult) HasWarnings() bool {
	return len(r.OrphanMeta) > 0
}

// MetaChecker checks Unity project meta file integrity.
type MetaChecker struct {
	project *Project
}

// NewMetaChecker creates a new MetaChecker.
func NewMetaChecker(project *Project) *MetaChecker {
	return &MetaChecker{project: project}
}

var excludedDirs = map[string]bool{
	".git":            true,
	"Library":         true,
	"Temp":            true,
	"Logs":            true,
	"obj":             true,
	"Build":           true,
	"Builds":          true,
	"UserSettings":    true,
	"ProjectSettings": true,
}

var excludedFiles = map[string]bool{
	".gitignore":         true,
	".gitattributes":     true,
	".DS_Store":          true,
	"Thumbs.db":          true,
	"manifest.json":      true,
	"packages-lock.json": true,
}

var metaRequiredRoots = map[string]bool{
	"Assets":   true,
	"Packages": true,
}

const metaFileFormatVersion = 2

type metaTemplate string

const (
	metaTemplateDefaultFile        metaTemplate = "default-file"
	metaTemplateDefaultFolder      metaTemplate = "default-folder"
	metaTemplateMono               metaTemplate = "mono"
	metaTemplateAssemblyDefinition metaTemplate = "assembly-definition"
	metaTemplateNativeFormat       metaTemplate = "native-format"
	metaTemplatePrefab             metaTemplate = "prefab"
	metaTemplateTextScript         metaTemplate = "text-script"
	metaTemplateShader             metaTemplate = "shader"
)

type MetaGenerateSkip struct {
	Path   string
	Reason string
}

type MetaGenerateResult struct {
	CreatedMeta   []string
	SkippedAssets []string
	Generated     []string
	Skipped       []MetaGenerateSkip
}

type MetaGenerateOptions struct {
	TargetPath string
	DryRun     bool
}

type MetaGenerator struct {
	project *Project
}

func NewMetaGenerator(project *Project) *MetaGenerator {
	return &MetaGenerator{project: project}
}

// Check performs meta file integrity check.
func (c *MetaChecker) Check() (*MetaCheckResult, error) {
	result := &MetaCheckResult{
		MissingMeta:    []string{},
		OrphanMeta:     []string{},
		DuplicateGUIDs: make(map[string][]string),
	}

	assets := make(map[string]bool)
	metas := make(map[string]bool)
	firstGUIDPaths := make(map[string]string)

	err := filepath.Walk(c.project.Path, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(c.project.Path, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		if info.IsDir() {
			if shouldExcludeDirectory(relPath) {
				return filepath.SkipDir
			}
			if isInsideMetaRequiredRoot(relPath) {
				assets[relPath] = true
			}
			return nil
		}

		if !isInsideMetaRequiredRoot(relPath) {
			return nil
		}
		if excludedFiles[filepath.Base(path)] {
			return nil
		}

		if strings.HasSuffix(relPath, ".meta") {
			metas[relPath] = true

			guid, err := extractGUID(path)
			if err != nil || guid == "" {
				return nil
			}

			if firstPath, exists := firstGUIDPaths[guid]; exists {
				if _, recorded := result.DuplicateGUIDs[guid]; !recorded {
					result.DuplicateGUIDs[guid] = []string{firstPath}
				}
				result.DuplicateGUIDs[guid] = append(result.DuplicateGUIDs[guid], relPath)
				return nil
			}

			firstGUIDPaths[guid] = relPath
			return nil
		}

		assets[relPath] = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk project directory: %w", err)
	}

	for asset := range assets {
		if !metas[asset+".meta"] {
			result.MissingMeta = append(result.MissingMeta, asset)
		}
	}
	sort.Strings(result.MissingMeta)

	for meta := range metas {
		assetPath := strings.TrimSuffix(meta, ".meta")
		if !assets[assetPath] {
			result.OrphanMeta = append(result.OrphanMeta, meta)
		}
	}
	sort.Strings(result.OrphanMeta)

	for guid, paths := range result.DuplicateGUIDs {
		sort.Strings(paths)
		result.DuplicateGUIDs[guid] = paths
	}

	return result, nil
}

// Fix removes orphan meta files and returns deleted files.
func (c *MetaChecker) Fix(dryRun bool) ([]string, error) {
	result, err := c.Check()
	if err != nil {
		return nil, err
	}

	deleted := make([]string, 0, len(result.OrphanMeta))
	for _, orphan := range result.OrphanMeta {
		deleted = append(deleted, orphan)
		if dryRun {
			continue
		}
		fullPath := filepath.Join(c.project.Path, filepath.FromSlash(orphan))
		if err := os.Remove(fullPath); err != nil {
			return deleted, fmt.Errorf("failed to remove %s: %w", orphan, err)
		}
	}

	return deleted, nil
}

// Generate creates missing .meta files for supported assets and folders.
func (c *MetaChecker) Generate(dryRun bool) (*MetaGenerateResult, error) {
	return NewMetaGenerator(c.project).Generate(MetaGenerateOptions{DryRun: dryRun})
}

// Generate creates missing .meta files for supported assets and folders.
func (g *MetaGenerator) Generate(opts MetaGenerateOptions) (*MetaGenerateResult, error) {
	checker := NewMetaChecker(g.project)
	checkResult, err := checker.Check()
	if err != nil {
		return nil, err
	}

	targetPath, err := normalizeTargetPath(g.project.Path, opts.TargetPath)
	if err != nil {
		return nil, err
	}

	missingAssets := append([]string{}, checkResult.MissingMeta...)
	sort.Strings(missingAssets)

	usedGUIDs, err := collectUsedGUIDs(g.project.Path)
	if err != nil {
		return nil, err
	}

	result := &MetaGenerateResult{
		CreatedMeta:   []string{},
		SkippedAssets: []string{},
		Generated:     []string{},
		Skipped:       []MetaGenerateSkip{},
	}

	matchedTarget := targetPath == ""

	for _, assetPath := range missingAssets {
		normalizedAsset := normalizeRelativePath(assetPath)
		if targetPath != "" && !pathMatchesTarget(normalizedAsset, targetPath) {
			continue
		}
		matchedTarget = true

		fullAssetPath := filepath.Join(g.project.Path, filepath.FromSlash(assetPath))
		info, err := os.Stat(fullAssetPath)
		if err != nil {
			result.SkippedAssets = append(result.SkippedAssets, assetPath)
			result.Skipped = append(result.Skipped, MetaGenerateSkip{
				Path:   normalizedAsset,
				Reason: fmt.Sprintf("asset unavailable: %v", err),
			})
			continue
		}

		template := selectMetaTemplate(assetPath, info.IsDir())
		if template == "" {
			result.SkippedAssets = append(result.SkippedAssets, assetPath)
			result.Skipped = append(result.Skipped, MetaGenerateSkip{
				Path:   normalizedAsset,
				Reason: unsupportedAssetReason(assetPath),
			})
			continue
		}

		metaPath := assetPath + ".meta"
		result.CreatedMeta = append(result.CreatedMeta, metaPath)
		result.Generated = append(result.Generated, normalizedAsset)

		if opts.DryRun {
			continue
		}

		guid, err := createUniqueGUID(usedGUIDs)
		if err != nil {
			return nil, err
		}

		if err := os.WriteFile(
			filepath.Join(g.project.Path, filepath.FromSlash(metaPath)),
			[]byte(renderMetaFile(template, guid)),
			0644,
		); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", metaPath, err)
		}
	}

	if targetPath != "" && !matchedTarget {
		reason := "no missing .meta files found"
		targetAbs := filepath.Join(g.project.Path, filepath.FromSlash(targetPath))
		if _, err := os.Stat(targetAbs); err != nil {
			reason = fmt.Sprintf("path not found: %v", err)
		}
		result.Skipped = append(result.Skipped, MetaGenerateSkip{
			Path:   targetPath,
			Reason: reason,
		})
	}

	return result, nil
}

func isInsideMetaRequiredRoot(relPath string) bool {
	relPath = filepath.Clean(relPath)
	parts := strings.SplitN(relPath, string(filepath.Separator), 2)
	if len(parts) < 2 {
		return false
	}
	return metaRequiredRoots[parts[0]]
}

func shouldExcludeDirectory(relPath string) bool {
	relPath = filepath.Clean(relPath)
	if filepath.Base(relPath) == ".git" {
		return true
	}
	return filepath.Dir(relPath) == "." && excludedDirs[relPath]
}

func extractGUID(metaPath string) (string, error) {
	file, err := os.Open(metaPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "guid:") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return "", nil
		}
		return strings.TrimSpace(parts[1]), nil
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func normalizeTargetPath(projectPath, target string) (string, error) {
	if target == "" {
		return "", nil
	}

	target = strings.TrimSuffix(target, ".meta")

	if filepath.IsAbs(target) {
		rel, err := filepath.Rel(projectPath, target)
		if err != nil {
			return "", fmt.Errorf("failed to resolve target path: %w", err)
		}
		target = rel
	}

	normalized := normalizeRelativePath(target)
	if normalized == "" || normalized == "." {
		return "", nil
	}
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("target path %s is outside the project", target)
	}

	return normalized, nil
}

func normalizeRelativePath(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	clean = filepath.ToSlash(clean)
	return strings.TrimPrefix(clean, "./")
}

func pathMatchesTarget(assetPath, target string) bool {
	if assetPath == target {
		return true
	}
	return strings.HasPrefix(assetPath, target+"/")
}

func unsupportedAssetReason(assetPath string) string {
	ext := strings.ToLower(filepath.Ext(assetPath))
	if ext == "" {
		ext = "unknown"
	}
	return fmt.Sprintf("unsupported asset type %s", ext)
}

func collectUsedGUIDs(projectPath string) (map[string]struct{}, error) {
	used := map[string]struct{}{}

	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(projectPath, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		if info.IsDir() {
			if shouldExcludeDirectory(relPath) {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(relPath, ".meta") || !isInsideMetaRequiredRoot(relPath) {
			return nil
		}

		guid, err := extractGUID(path)
		if err != nil || guid == "" {
			return nil
		}
		used[guid] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan existing GUIDs: %w", err)
	}

	return used, nil
}

func selectMetaTemplate(assetPath string, isDirectory bool) metaTemplate {
	if isDirectory {
		return metaTemplateDefaultFolder
	}

	ext := strings.ToLower(filepath.Ext(assetPath))
	switch ext {
	case ".cs":
		return metaTemplateMono
	case ".asmdef":
		return metaTemplateAssemblyDefinition
	case ".prefab":
		return metaTemplatePrefab
	case ".asset", ".mat":
		return metaTemplateNativeFormat
	case ".unity":
		return metaTemplateDefaultFile
	case ".shader":
		return metaTemplateShader
	}

	if isTextScriptExtension(ext) || filepath.Base(assetPath) == "package.json" {
		return metaTemplateTextScript
	}

	return ""
}

func isTextScriptExtension(ext string) bool {
	switch ext {
	case ".txt", ".md", ".json", ".xml", ".yaml", ".yml", ".csv", ".asmref", ".rsp", ".cginc", ".compute", ".hlsl", ".uss", ".uxml":
		return true
	default:
		return false
	}
}

func createUniqueGUID(usedGUIDs map[string]struct{}) (string, error) {
	for {
		buffer := make([]byte, 16)
		if _, err := rand.Read(buffer); err != nil {
			return "", fmt.Errorf("failed to generate GUID: %w", err)
		}

		candidate := hex.EncodeToString(buffer)
		if _, exists := usedGUIDs[candidate]; exists {
			continue
		}

		usedGUIDs[candidate] = struct{}{}
		return candidate, nil
	}
}

func renderMetaFile(template metaTemplate, guid string) string {
	lines := []string{
		fmt.Sprintf("fileFormatVersion: %d", metaFileFormatVersion),
		fmt.Sprintf("guid: %s", guid),
	}

	switch template {
	case metaTemplateDefaultFolder:
		lines = append(lines,
			"folderAsset: yes",
			"DefaultImporter:",
			"  externalObjects: {}",
			"  userData: ",
			"  assetBundleName: ",
			"  assetBundleVariant: ",
		)
	case metaTemplateDefaultFile:
		lines = append(lines,
			"DefaultImporter:",
			"  externalObjects: {}",
			"  userData: ",
			"  assetBundleName: ",
			"  assetBundleVariant: ",
		)
	case metaTemplateMono:
		lines = append(lines,
			"MonoImporter:",
			"  externalObjects: {}",
			"  serializedVersion: 2",
			"  defaultReferences: []",
			"  executionOrder: 0",
			"  icon: {instanceID: 0}",
			"  userData: ",
			"  assetBundleName: ",
			"  assetBundleVariant: ",
		)
	case metaTemplateAssemblyDefinition:
		lines = append(lines,
			"AssemblyDefinitionImporter:",
			"  externalObjects: {}",
			"  userData: ",
			"  assetBundleName: ",
			"  assetBundleVariant: ",
		)
	case metaTemplatePrefab:
		lines = append(lines,
			"PrefabImporter:",
			"  externalObjects: {}",
			"  userData: ",
			"  assetBundleName: ",
			"  assetBundleVariant: ",
		)
	case metaTemplateNativeFormat:
		lines = append(lines,
			"NativeFormatImporter:",
			"  externalObjects: {}",
			"  mainObjectFileID: 0",
			"  userData: ",
			"  assetBundleName: ",
			"  assetBundleVariant: ",
		)
	case metaTemplateTextScript:
		lines = append(lines,
			"TextScriptImporter:",
			"  externalObjects: {}",
			"  userData: ",
			"  assetBundleName: ",
			"  assetBundleVariant: ",
		)
	case metaTemplateShader:
		lines = append(lines,
			"ShaderImporter:",
			"  externalObjects: {}",
			"  defaultTextures: []",
			"  nonModifiableTextures: []",
			"  userData: ",
			"  assetBundleName: ",
			"  assetBundleVariant: ",
		)
	}

	return strings.Join(append(lines, ""), "\n")
}
