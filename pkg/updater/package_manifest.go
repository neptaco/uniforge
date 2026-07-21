package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PackageManifestLoader loads package.json from a package path at a Git tag.
type PackageManifestLoader func(ctx context.Context, repositoryURL, packagePath, tag string) ([]byte, error)

// LoadPackageManifestFromGit fetches package.json from a tagged Git package
// without checking out the repository worktree.
func LoadPackageManifestFromGit(
	ctx context.Context,
	repositoryURL string,
	packagePath string,
	tag string,
) ([]byte, error) {
	if repositoryURL == "" || packagePath == "" {
		return nil, fmt.Errorf("package source does not identify a repository and package path")
	}

	temporaryDirectory, err := os.MkdirTemp("", "uniforge-package-check-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary package directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(temporaryDirectory) }()

	clone := exec.CommandContext(
		ctx,
		"git",
		"clone",
		"--quiet",
		"--filter=blob:none",
		"--no-checkout",
		"--depth",
		"1",
		"--branch",
		tag,
		"--single-branch",
		"--",
		repositoryURL,
		temporaryDirectory,
	)
	clone.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := clone.CombinedOutput(); err != nil {
		return nil, packageManifestCommandError("fetch package source", output, err)
	}

	packageManifestPath := filepath.ToSlash(filepath.Join(packagePath, "package.json"))
	show := exec.CommandContext(ctx, "git", "-C", temporaryDirectory, "show", "HEAD:"+packageManifestPath)
	manifestData, err := show.CombinedOutput()
	if err != nil {
		return nil, packageManifestCommandError("read package.json from package source", manifestData, err)
	}
	return manifestData, nil
}

func packageManifestCommandError(action string, output []byte, err error) error {
	detail := strings.Join(strings.Fields(string(output)), " ")
	if detail == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	const maximumDetailLength = 500
	if len(detail) > maximumDetailLength {
		detail = detail[:maximumDetailLength] + "..."
	}
	return fmt.Errorf("%s: %s: %w", action, detail, err)
}
