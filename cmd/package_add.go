package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/updater"
	"github.com/spf13/cobra"
)

var (
	githubSourcePartPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	packagePathPattern      = regexp.MustCompile(`^[A-Za-z0-9_.~/-]+$`)
	gitTagPattern           = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	packageAddIsInteractive = func(cmd *cobra.Command) bool {
		input, inputOK := cmd.InOrStdin().(interface{ Fd() uintptr })
		output, outputOK := cmd.OutOrStdout().(interface{ Fd() uintptr })
		if !inputOK || !outputOK {
			return false
		}
		return isTerminal(input.Fd()) && isTerminal(output.Fd())
	}
)

type packageSource struct {
	packageID       string
	manifestURL     string
	repositoryURL   string
	packagePath     string
	githubAPIBase   string
	originalDisplay string
}

func runPackageAdd(cmd *cobra.Command, args []string) error {
	projectArg, sourceArg := packageAddArguments(args)
	source, err := parsePackageSource(sourceArg)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}
	_, projectPath, err := bridge.ResolveProjectOrPath(projectArg, cwd, nil)
	if err != nil {
		return err
	}
	if projectPath == "" {
		return fmt.Errorf("no Unity project found; specify a project path")
	}

	tag, err := resolvePackageAddTag(cmd.Context(), source, packageAddTag)
	if err != nil {
		return err
	}
	resolvedSource, compatibility, compatibilityErr := inspectPackageAddCompatibility(
		cmd.Context(),
		projectPath,
		source,
		tag,
	)
	if resolvedSource.packageID != "" {
		source = resolvedSource
	}
	if compatibilityErr != nil {
		if !packageAddForce {
			return fmt.Errorf("%w; use --force to add the package anyway", compatibilityErr)
		}
		compatibility.forced = true
		compatibility.warning = compatibilityErr.Error()
	}

	confirmed, err := confirmPackageAdd(cmd, projectPath, source, tag, compatibility)
	if err != nil {
		return err
	}
	if !confirmed {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes were made.")
		return err
	}
	if err := addPackageToProject(projectPath, source, tag); err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		cmd.OutOrStdout(),
		"Package added: %s (%s#%s)\n",
		source.packageID,
		source.manifestURL,
		tag,
	)
	return err
}

func packageAddArguments(args []string) (projectArg, sourceArg string) {
	if len(args) == 1 {
		return "", args[0]
	}
	return args[0], args[1]
}

func isTerminal(fd uintptr) bool {
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func confirmPackageAdd(
	cmd *cobra.Command,
	projectPath string,
	source packageSource,
	tag string,
	compatibility packageAddCompatibility,
) (bool, error) {
	if packageAddYes || !packageAddIsInteractive(cmd) {
		return true, nil
	}

	projectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return false, fmt.Errorf("resolve project path: %w", err)
	}
	manifestPath := filepath.Join(projectPath, "Packages", "manifest.json")
	reference := source.manifestURL + "#" + tag
	compatibilityText := compatibility.summary()
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"Package to add:\n  Project: %s\n  Project Unity: %s\n  Package: %s\n  Package Unity: %s\n  Compatibility: %s\n  Source: %s\n  Tag: %s\n  Reference: %s\n  Manifest: %s\n\nAdd this package? [Y/n]: ",
		projectPath,
		compatibility.projectDisplay(),
		source.packageID,
		compatibility.packageDisplay(),
		compatibilityText,
		source.manifestURL,
		tag,
		reference,
		manifestPath,
	); err != nil {
		return false, err
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	for {
		response, readErr := reader.ReadString('\n')
		response = strings.ToLower(strings.TrimSpace(response))
		switch response {
		case "", "y", "yes":
			if readErr != nil && response == "" {
				return false, nil
			}
			return true, nil
		case "n", "no":
			return false, nil
		}
		if readErr != nil {
			if readErr == io.EOF {
				return false, nil
			}
			return false, fmt.Errorf("read confirmation: %w", readErr)
		}
		if _, err := fmt.Fprint(cmd.OutOrStdout(), "Enter yes or no [Y/n]: "); err != nil {
			return false, err
		}
	}
}

func parsePackageSource(value string) (packageSource, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return packageSource{}, fmt.Errorf("package source is empty")
	}
	if strings.Contains(value, "#") {
		return packageSource{}, fmt.Errorf("package source must not contain a fragment; pass the tag with --tag")
	}
	if strings.Contains(value, "://") {
		return parsePackageURL(value)
	}
	return parseGitHubPackageSource(value)
}

func parsePackageURL(value string) (packageSource, error) {
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
		return packageSource{}, fmt.Errorf("package source must be an HTTPS Git URL or GitHub shorthand")
	}
	if parsed.User != nil {
		return packageSource{}, fmt.Errorf("package source must not contain credentials")
	}
	if !strings.HasSuffix(strings.ToLower(parsed.Path), ".git") {
		return packageSource{}, fmt.Errorf("package URL must end in .git")
	}
	packagePath := strings.Trim(parsed.Query().Get("path"), "/")
	packageID, err := packageIDFromPath(packagePath)
	if err != nil {
		return packageSource{}, err
	}

	source := packageSource{
		packageID:       packageID,
		manifestURL:     value,
		repositoryURL:   gitRepositoryURL(parsed),
		packagePath:     packagePath,
		originalDisplay: value,
	}
	if strings.EqualFold(parsed.Hostname(), "github.com") {
		owner, repository, ok := githubRepositoryFromURLPath(parsed.Path)
		if !ok {
			return packageSource{}, fmt.Errorf("GitHub package URL must identify an owner and repository")
		}
		source.githubAPIBase = githubAPIBase(owner, repository)
	}
	return source, nil
}

func parseGitHubPackageSource(value string) (packageSource, error) {
	value = strings.TrimPrefix(value, "github:")
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) < 3 {
		return packageSource{}, fmt.Errorf(
			"GitHub shorthand must be owner/repository/path/to/package",
		)
	}
	owner := parts[0]
	repository := strings.TrimSuffix(parts[1], ".git")
	packagePath := strings.Join(parts[2:], "/")
	if !githubSourcePartPattern.MatchString(owner) || !githubSourcePartPattern.MatchString(repository) {
		return packageSource{}, fmt.Errorf("GitHub shorthand contains an invalid owner or repository")
	}
	packageID, err := packageIDFromPath(packagePath)
	if err != nil {
		return packageSource{}, err
	}

	return packageSource{
		packageID:       packageID,
		manifestURL:     fmt.Sprintf("https://github.com/%s/%s.git?path=%s", owner, repository, packagePath),
		repositoryURL:   fmt.Sprintf("https://github.com/%s/%s.git", owner, repository),
		packagePath:     packagePath,
		githubAPIBase:   githubAPIBase(owner, repository),
		originalDisplay: value,
	}, nil
}

func gitRepositoryURL(parsed *url.URL) string {
	repository := *parsed
	repository.RawQuery = ""
	repository.Fragment = ""
	return repository.String()
}

func packageIDFromPath(packagePath string) (string, error) {
	if packagePath == "" || !packagePathPattern.MatchString(packagePath) {
		return "", fmt.Errorf("package source must include a valid ?path=... package path")
	}
	packageID := pathpkg.Base(pathpkg.Clean(packagePath))
	if packageID == "." || packageID == "/" {
		return "", fmt.Errorf("package source must identify a package directory")
	}
	return packageID, nil
}

func githubRepositoryFromURLPath(value string) (string, string, bool) {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) != 2 {
		return "", "", false
	}
	owner := parts[0]
	repository := strings.TrimSuffix(parts[1], ".git")
	if !githubSourcePartPattern.MatchString(owner) || !githubSourcePartPattern.MatchString(repository) {
		return "", "", false
	}
	return owner, repository, true
}

func githubAPIBase(owner, repository string) string {
	return fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repository)
}

func resolvePackageAddTag(ctx context.Context, source packageSource, requestedTag string) (string, error) {
	requestedTag = strings.TrimSpace(requestedTag)
	if requestedTag != "" {
		if !gitTagPattern.MatchString(requestedTag) {
			return "", fmt.Errorf("invalid git tag %q", requestedTag)
		}
		return requestedTag, nil
	}
	if source.githubAPIBase == "" {
		return "", fmt.Errorf("cannot resolve a tag automatically for %s; specify --tag", source.originalDisplay)
	}

	refreshContext, cancel := context.WithTimeout(ctx, packageVersionRefreshTimeout)
	defer cancel()
	tag, err := updater.ResolveLatestGitHubTag(refreshContext, source.githubAPIBase, http.DefaultClient)
	if err != nil {
		return "", fmt.Errorf("resolve latest package tag: %w", err)
	}
	return tag, nil
}

func addPackageToProject(projectPath string, source packageSource, tag string) error {
	manifestPath := filepath.Join(projectPath, "Packages", "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}

	updatedManifest, err := rewritePackageManifestForAdd(manifestData, source, tag)
	if err != nil {
		return err
	}
	if err := writeFileAtomically(manifestPath, updatedManifest); err != nil {
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}
	return nil
}

func rewritePackageManifestForAdd(data []byte, source packageSource, tag string) ([]byte, error) {
	if !gitTagPattern.MatchString(tag) {
		return nil, fmt.Errorf("invalid git tag %q", tag)
	}

	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode Packages/manifest.json: %w", err)
	}

	rawDependencies, exists := manifest["dependencies"]
	if !exists {
		return nil, fmt.Errorf("manifest.json does not contain dependencies")
	}
	var dependencies map[string]string
	if err := json.Unmarshal(rawDependencies, &dependencies); err != nil {
		return nil, fmt.Errorf("decode Packages/manifest.json dependencies: %w", err)
	}
	if dependencies == nil {
		return nil, fmt.Errorf("manifest.json dependencies must be an object")
	}
	if _, exists := dependencies[source.packageID]; exists {
		return nil, fmt.Errorf(
			"package %s is already present in Packages/manifest.json",
			source.packageID,
		)
	}

	dependencies[source.packageID] = source.manifestURL + "#" + tag
	encodedDependencies, err := json.Marshal(dependencies)
	if err != nil {
		return nil, err
	}
	manifest["dependencies"] = encodedDependencies
	return marshalIndentedJSON(manifest)
}
