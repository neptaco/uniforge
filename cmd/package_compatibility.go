package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const packageCompatibilityCheckTimeout = 60 * time.Second

var (
	projectUnityVersionPattern = regexp.MustCompile(`(?m)^m_EditorVersion:\s*(\S+)\s*$`)
	unityMajorMinorPattern     = regexp.MustCompile(`^(\d+)\.(\d+)$`)
	unityFullVersionPattern    = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)([abfp])(\d+)$`)
	unityReleasePattern        = regexp.MustCompile(`^(\d+)([abfp])(\d+)$`)
	packageManifestLoader      = loadPackageManifestFromGit
)

type packageManifestCompatibility struct {
	Name         string `json:"name"`
	Unity        string `json:"unity"`
	UnityRelease string `json:"unityRelease"`
}

type packageCompatibility struct {
	projectVersion  string
	minimumUnity    string
	requirementRead bool
	forced          bool
	warning         string
}

func (compatibility packageCompatibility) projectDisplay() string {
	if compatibility.projectVersion == "" {
		return "unknown"
	}
	return compatibility.projectVersion
}

func (compatibility packageCompatibility) packageDisplay() string {
	if !compatibility.requirementRead {
		return "unknown"
	}
	if compatibility.minimumUnity == "" {
		return "no minimum declared"
	}
	return compatibility.minimumUnity + " or later"
}

func (compatibility packageCompatibility) summary() string {
	if compatibility.forced {
		if compatibility.warning == "" {
			return "check skipped (--force)"
		}
		return "forced despite warning: " + compatibility.warning
	}
	return "compatible"
}

type parsedUnityVersion struct {
	major          int
	minor          int
	update         int
	channel        int
	channelVersion int
}

func inspectPackageCompatibility(
	ctx context.Context,
	projectPath string,
	source packageSource,
	tag string,
) (packageSource, packageCompatibility, error) {
	compatibility := packageCompatibility{}
	projectVersionPath := filepath.Join(projectPath, "ProjectSettings", "ProjectVersion.txt")
	projectVersionData, err := os.ReadFile(projectVersionPath)
	if err != nil {
		return source, compatibility, fmt.Errorf("read %s: %w", projectVersionPath, err)
	}
	projectVersion, projectDisplay, err := parseProjectUnityVersion(projectVersionData)
	if err != nil {
		return source, compatibility, err
	}
	compatibility.projectVersion = projectDisplay

	checkContext, cancel := context.WithTimeout(ctx, packageCompatibilityCheckTimeout)
	defer cancel()
	manifestData, err := packageManifestLoader(checkContext, source, tag)
	if err != nil {
		return source, compatibility, fmt.Errorf("verify package manifest: %w", err)
	}

	var manifest packageManifestCompatibility
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return source, packageCompatibility{}, fmt.Errorf("decode package.json: %w", err)
	}
	manifest.Name = strings.TrimSpace(manifest.Name)
	if manifest.Name == "" {
		return source, packageCompatibility{}, fmt.Errorf("package.json does not declare a package name")
	}
	source.packageID = manifest.Name

	minimumVersion, minimumDisplay, err := parsePackageMinimumUnity(manifest.Unity, manifest.UnityRelease)
	if err != nil {
		return source, compatibility, fmt.Errorf("read package Unity requirement: %w", err)
	}
	compatibility.minimumUnity = minimumDisplay
	compatibility.requirementRead = true

	if minimumVersion != nil && compareUnityVersions(projectVersion, *minimumVersion) < 0 {
		return source, compatibility, fmt.Errorf(
			"package %s requires Unity %s or later, but the project uses Unity %s",
			source.packageID,
			minimumDisplay,
			projectDisplay,
		)
	}
	return source, compatibility, nil
}

func loadPackageManifestFromGit(ctx context.Context, source packageSource, tag string) ([]byte, error) {
	if source.repositoryURL == "" || source.packagePath == "" {
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
		source.repositoryURL,
		temporaryDirectory,
	)
	clone.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := clone.CombinedOutput(); err != nil {
		return nil, commandError("fetch package source", output, err)
	}

	packageManifestPath := filepath.ToSlash(filepath.Join(source.packagePath, "package.json"))
	show := exec.CommandContext(ctx, "git", "-C", temporaryDirectory, "show", "HEAD:"+packageManifestPath)
	manifestData, err := show.CombinedOutput()
	if err != nil {
		return nil, commandError("read package.json from package source", manifestData, err)
	}
	return manifestData, nil
}

func commandError(action string, output []byte, err error) error {
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

func parseProjectUnityVersion(data []byte) (parsedUnityVersion, string, error) {
	match := projectUnityVersionPattern.FindSubmatch(data)
	if len(match) != 2 {
		return parsedUnityVersion{}, "", fmt.Errorf("ProjectSettings/ProjectVersion.txt does not contain m_EditorVersion")
	}
	display := string(match[1])
	version, err := parseUnityFullVersion(display)
	if err != nil {
		return parsedUnityVersion{}, "", fmt.Errorf("parse project Unity version %q: %w", display, err)
	}
	return version, display, nil
}

func parsePackageMinimumUnity(unityValue, unityRelease string) (*parsedUnityVersion, string, error) {
	unityValue = strings.TrimSpace(unityValue)
	unityRelease = strings.TrimSpace(unityRelease)
	if unityValue == "" {
		return nil, "", nil
	}

	minimum, err := parseUnityMajorMinor(unityValue)
	if err != nil {
		return nil, "", fmt.Errorf("invalid unity value %q; expected major.minor", unityValue)
	}
	if unityRelease == "" {
		return &minimum, unityValue, nil
	}

	releaseMatch := unityReleasePattern.FindStringSubmatch(unityRelease)
	if len(releaseMatch) != 4 {
		return nil, "", fmt.Errorf("invalid unityRelease value %q", unityRelease)
	}
	minimum.update, _ = strconv.Atoi(releaseMatch[1])
	minimum.channel = unityChannelRank(releaseMatch[2])
	minimum.channelVersion, _ = strconv.Atoi(releaseMatch[3])
	return &minimum, unityValue + "." + unityRelease, nil
}

func parseUnityMajorMinor(value string) (parsedUnityVersion, error) {
	match := unityMajorMinorPattern.FindStringSubmatch(value)
	if len(match) != 3 {
		return parsedUnityVersion{}, fmt.Errorf("invalid Unity version %q; expected major.minor", value)
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	return parsedUnityVersion{major: major, minor: minor}, nil
}

func parseUnityFullVersion(value string) (parsedUnityVersion, error) {
	match := unityFullVersionPattern.FindStringSubmatch(value)
	if len(match) != 6 {
		return parsedUnityVersion{}, fmt.Errorf("expected major.minor.update followed by a, b, f, or p release")
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	update, _ := strconv.Atoi(match[3])
	channelVersion, _ := strconv.Atoi(match[5])
	return parsedUnityVersion{
		major:          major,
		minor:          minor,
		update:         update,
		channel:        unityChannelRank(match[4]),
		channelVersion: channelVersion,
	}, nil
}

func unityChannelRank(channel string) int {
	switch channel {
	case "a":
		return 0
	case "b":
		return 1
	case "f":
		return 2
	case "p":
		return 3
	default:
		return -1
	}
}

func compareUnityVersions(left, right parsedUnityVersion) int {
	leftParts := [...]int{left.major, left.minor, left.update, left.channel, left.channelVersion}
	rightParts := [...]int{right.major, right.minor, right.update, right.channel, right.channelVersion}
	for index := range leftParts {
		if leftParts[index] < rightParts[index] {
			return -1
		}
		if leftParts[index] > rightParts[index] {
			return 1
		}
	}
	return 0
}
