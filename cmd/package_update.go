package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/daemon"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/neptaco/uniforge/pkg/updater"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	unityPackageID                 = "dev.crysta.uniforge"
	packageUpdateToolName          = "package-update"
	packageVersionRefreshTimeout   = 10 * time.Second
	packageUpdateToolTimeout       = 30 * time.Second
	packageUpdatePollInterval      = 2 * time.Second
	packageUpdateCompletionTimeout = 180 * time.Second
)

var (
	packageAddTag        string
	packageAddYes        bool
	packageAddForce      bool
	packageUpdateVersion string
	packageUpdateNoWait  bool

	barePackageVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

var packageCmd = &cobra.Command{
	Use:   "package",
	Short: "Manage Unity project packages",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var packageUpdateCmd = &cobra.Command{
	Use:   "update [project]",
	Short: "Update the UniForge package in a Unity project",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPackageUpdate,
}

var packageAddCmd = &cobra.Command{
	Use:   "add [project] <package-source>",
	Short: "Add a Git package to a Unity project",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runPackageAdd,
}

type packageVersionResolverDeps struct {
	prepare func(updater.AutoCheckOptions) (updater.AutoCheckDecision, error)
	refresh func(context.Context, updater.AutoCheckOptions) error
	read    func(string) (string, error)
}

func init() {
	rootCmd.AddCommand(packageCmd)
	packageCmd.AddCommand(packageAddCmd)
	packageCmd.AddCommand(packageUpdateCmd)

	packageAddCmd.Flags().StringVar(&packageAddTag, "tag", "", "git tag (default: latest semantic-version tag on GitHub)")
	packageAddCmd.Flags().BoolVarP(&packageAddYes, "yes", "y", false, "skip the interactive confirmation")
	packageAddCmd.Flags().BoolVar(&packageAddForce, "force", false, "add even when the Unity compatibility check fails")
	packageUpdateCmd.Flags().StringVar(&packageUpdateVersion, "version", "", "target package version (X.Y.Z)")
	packageUpdateCmd.Flags().BoolVar(&packageUpdateNoWait, "no-wait", false, "return after the package update starts")
}

func runPackageUpdate(cmd *cobra.Command, args []string) error {
	targetVersion, err := resolveRequestedPackageVersion(cmd.Context(), packageUpdateVersion)
	if err != nil {
		return err
	}

	client := newToolClient(toolClientOptions{
		timeoutMS:       int((5 * time.Second).Milliseconds()),
		autoStartDaemon: false,
	})
	defer func() { _ = client.Close() }()

	projects, connectedToDaemon, err := packageUpdateProjects(client)
	if err != nil {
		return err
	}

	projectArg := ""
	if len(args) > 0 {
		projectArg = args[0]
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}
	connectedProject, offlineProjectPath, err := bridge.ResolveProjectOrPath(projectArg, cwd, projects)
	if err != nil {
		return err
	}
	if connectedProject == nil && offlineProjectPath == "" {
		return fmt.Errorf("no Unity project found; specify a project path")
	}

	if connectedProject != nil {
		if !connectedToDaemon {
			return fmt.Errorf("resolved connected project without a daemon connection")
		}
		return updateConnectedPackage(cmd, client, connectedProject, targetVersion)
	}

	connectedProject, err = recheckOfflinePackageConnection(client, connectedToDaemon, offlineProjectPath)
	if err != nil {
		return err
	}
	if connectedProject != nil {
		return updateConnectedPackage(cmd, client, connectedProject, targetVersion)
	}

	if err := updateOfflinePackageFiles(offlineProjectPath, targetVersion); err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		cmd.OutOrStdout(),
		"Manifest updated; Unity will resolve v%s on next editor start\n",
		targetVersion,
	)
	return err
}

func resolveRequestedPackageVersion(ctx context.Context, requestedVersion string) (string, error) {
	versionOptions := updater.AutoCheckOptions{}
	if strings.TrimSpace(requestedVersion) == "" {
		var err error
		versionOptions, err = unityPackageAutoCheckOptions()
		if err != nil {
			return "", fmt.Errorf("locate Unity package version cache: %w", err)
		}
	}

	return resolvePackageUpdateVersion(
		ctx,
		requestedVersion,
		versionOptions,
		packageVersionResolverDeps{
			prepare: updater.PrepareUnityPackageAutoCheck,
			refresh: updater.RefreshUnityPackageAutoCheck,
			read:    updater.ReadUnityPackageLatestVersion,
		},
	)
}

func recheckOfflinePackageConnection(
	client *bridge.Client,
	connectedToDaemon bool,
	projectPath string,
) (*bridge.ProjectInfo, error) {
	var projects []bridge.ProjectInfo
	if connectedToDaemon {
		result, err := client.ListProjects(false)
		if err != nil {
			return nil, fmt.Errorf("recheck connected Unity projects: %w", err)
		}
		projects = result.Projects
	} else {
		var err error
		projects, connectedToDaemon, err = packageUpdateProjects(client)
		if err != nil {
			return nil, err
		}
		if !connectedToDaemon {
			return nil, nil
		}
	}

	return bridge.MatchProject(bridge.CwdHints{ProjectPath: projectPath}, projects), nil
}

func packageUpdateProjects(client *bridge.Client) ([]bridge.ProjectInfo, bool, error) {
	if err := client.Connect(); err != nil {
		if daemon.IsRunning(daemonConfig()) {
			return nil, false, fmt.Errorf("connect to running daemon: %w", err)
		}
		return nil, false, nil
	}
	if _, err := client.Register(); err != nil {
		return nil, true, fmt.Errorf("register package update client: %w", err)
	}
	result, err := client.ListProjects(false)
	if err != nil {
		return nil, true, fmt.Errorf("list connected Unity projects: %w", err)
	}
	return result.Projects, true, nil
}

func updateConnectedPackage(
	cmd *cobra.Command,
	client *bridge.Client,
	project *bridge.ProjectInfo,
	targetVersion string,
) error {
	oldVersion := project.PackageVersion
	if oldVersion == targetVersion {
		_, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"Package is already up to date (%s)\n",
			targetVersion,
		)
		return err
	}

	result, err := client.ToolCall(
		packageUpdateToolName,
		map[string]any{"version": targetVersion},
		project.ID,
		packageUpdateToolTimeout,
	)
	if err != nil {
		return fmt.Errorf("start package update: %w", err)
	}
	if !result.Success {
		return toolCallFailureError(packageUpdateToolName, result)
	}
	if result.Result == nil {
		return fmt.Errorf("tool %s returned an empty started acknowledgement", packageUpdateToolName)
	}
	if err := writePackageUpdateAcknowledgement(cmd.OutOrStdout(), result.Result); err != nil {
		return fmt.Errorf("write package update acknowledgement: %w", err)
	}
	if packageUpdateNoWait {
		return nil
	}

	reached, err := waitForPackageUpdate(
		cmd.Context(),
		client,
		project.ID,
		targetVersion,
		packageUpdatePollInterval,
		packageUpdateCompletionTimeout,
	)
	if err != nil {
		return err
	}
	if !reached {
		_, err = fmt.Fprintf(
			cmd.ErrOrStderr(),
			"Note: Package update was started, but completion was not observed. Check the Editor-side package resolve (uniforge logs %q).\n",
			project.ID,
		)
		return err
	}

	_, err = fmt.Fprintf(
		cmd.OutOrStdout(),
		"Package updated: %s -> %s\n",
		oldVersion,
		targetVersion,
	)
	return err
}

func writePackageUpdateAcknowledgement(writer io.Writer, acknowledgement any) error {
	data, err := yaml.Marshal(acknowledgement)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

type packageProjectLister interface {
	ListProjects(includeTools bool) (*bridge.ClientListProjectsResult, error)
}

func waitForPackageUpdate(
	ctx context.Context,
	lister packageProjectLister,
	projectID string,
	targetVersion string,
	pollInterval time.Duration,
	timeout time.Duration,
) (bool, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-timer.C:
			return false, nil
		case <-ticker.C:
			if !time.Now().Before(deadline) {
				return false, nil
			}
			result, err := lister.ListProjects(false)
			if err != nil {
				continue
			}
			if !time.Now().Before(deadline) {
				return false, nil
			}
			if packageUpdateReached(result.Projects, projectID, targetVersion) {
				return true, nil
			}
		}
	}
}

func packageUpdateReached(projects []bridge.ProjectInfo, projectID, targetVersion string) bool {
	for _, project := range projects {
		if project.ID == projectID && project.PackageVersion == targetVersion {
			return true
		}
	}
	return false
}

func resolvePackageUpdateVersion(
	ctx context.Context,
	requestedVersion string,
	opts updater.AutoCheckOptions,
	deps packageVersionResolverDeps,
) (string, error) {
	requestedVersion = strings.TrimSpace(requestedVersion)
	if requestedVersion != "" {
		return validatePackageUpdateVersion(requestedVersion)
	}

	decision, err := deps.prepare(opts)
	if err != nil {
		return "", fmt.Errorf("read Unity package update cache: %w", err)
	}
	if decision.LatestVersion != "" {
		return validatePackageUpdateVersion(decision.LatestVersion)
	}

	refreshContext, cancel := context.WithTimeout(ctx, packageVersionRefreshTimeout)
	refreshErr := deps.refresh(refreshContext, opts)
	cancel()

	latestVersion, err := deps.read(opts.CachePath)
	if err != nil {
		return "", fmt.Errorf("read refreshed Unity package update cache: %w", err)
	}
	if latestVersion != "" {
		return validatePackageUpdateVersion(latestVersion)
	}
	if refreshErr != nil {
		return "", fmt.Errorf(
			"latest Unity package version is unavailable; specify --version X.Y.Z: %w",
			refreshErr,
		)
	}
	return "", fmt.Errorf("latest Unity package version is unavailable; specify --version X.Y.Z")
}

func validatePackageUpdateVersion(version string) (string, error) {
	if !barePackageVersionPattern.MatchString(version) {
		return "", fmt.Errorf("invalid package version %q; expected X.Y.Z", version)
	}
	return version, nil
}

func rewritePackageManifestVersion(data []byte, targetVersion string) ([]byte, error) {
	if _, err := validatePackageUpdateVersion(targetVersion); err != nil {
		return nil, err
	}

	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode Packages/manifest.json: %w", err)
	}

	rawDependencies, exists := manifest["dependencies"]
	if !exists {
		return nil, fmt.Errorf("package %s is not present in Packages/manifest.json", unityPackageID)
	}
	var dependencies map[string]string
	if err := json.Unmarshal(rawDependencies, &dependencies); err != nil {
		return nil, fmt.Errorf("decode Packages/manifest.json dependencies: %w", err)
	}
	reference, exists := dependencies[unityPackageID]
	if !exists {
		return nil, fmt.Errorf("package %s is not present in Packages/manifest.json", unityPackageID)
	}

	updatedReference, err := packageGitReferenceAtVersion(reference, targetVersion)
	if err != nil {
		return nil, err
	}
	dependencies[unityPackageID] = updatedReference
	encodedDependencies, err := json.Marshal(dependencies)
	if err != nil {
		return nil, err
	}
	manifest["dependencies"] = encodedDependencies
	return marshalIndentedJSON(manifest)
}

func packageGitReferenceAtVersion(reference, targetVersion string) (string, error) {
	parsed, err := url.Parse(reference)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" ||
		!strings.HasSuffix(strings.ToLower(parsed.Path), ".git") || parsed.Query().Get("path") == "" {
		return "", fmt.Errorf(
			"package %s must use an HTTPS git URL with ?path=...; got %q",
			unityPackageID,
			reference,
		)
	}

	fragmentlessReference := reference
	if fragmentIndex := strings.IndexByte(fragmentlessReference, '#'); fragmentIndex >= 0 {
		fragmentlessReference = fragmentlessReference[:fragmentIndex]
	}
	return fragmentlessReference + "#v" + targetVersion, nil
}

func removePackageLockEntry(data []byte) ([]byte, bool, error) {
	var lock map[string]json.RawMessage
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, false, fmt.Errorf("decode Packages/packages-lock.json: %w", err)
	}

	rawDependencies, exists := lock["dependencies"]
	if !exists {
		return data, false, nil
	}
	var dependencies map[string]json.RawMessage
	if err := json.Unmarshal(rawDependencies, &dependencies); err != nil {
		return nil, false, fmt.Errorf("decode Packages/packages-lock.json dependencies: %w", err)
	}
	if _, exists := dependencies[unityPackageID]; !exists {
		return data, false, nil
	}

	delete(dependencies, unityPackageID)
	encodedDependencies, err := json.Marshal(dependencies)
	if err != nil {
		return nil, false, err
	}
	lock["dependencies"] = encodedDependencies
	updated, err := marshalIndentedJSON(lock)
	if err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

func marshalIndentedJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func updateOfflinePackageFiles(projectPath, targetVersion string) error {
	manifestPath := filepath.Join(projectPath, "Packages", "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}
	updatedManifest, err := rewritePackageManifestVersion(manifestData, targetVersion)
	if err != nil {
		return err
	}

	lockPath := filepath.Join(projectPath, "Packages", "packages-lock.json")
	var updatedLock []byte
	lockChanged := false
	lockData, lockReadErr := os.ReadFile(lockPath)
	if lockReadErr == nil {
		updatedLock, lockChanged, err = removePackageLockEntry(lockData)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(lockReadErr) {
		return fmt.Errorf("read %s: %w", lockPath, lockReadErr)
	}

	if err := unity.NewEditor("").CheckNotRunning(projectPath); err != nil {
		return fmt.Errorf("refusing offline package update: %w", err)
	}
	if lockChanged {
		if err := writeFileAtomically(lockPath, updatedLock); err != nil {
			return fmt.Errorf("write %s: %w", lockPath, err)
		}
	}
	if err := unity.NewEditor("").CheckNotRunning(projectPath); err != nil {
		return fmt.Errorf("refusing offline package update: %w", err)
	}
	if err := writeFileAtomically(manifestPath, updatedManifest); err != nil {
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}
	return nil
}

func writeFileAtomically(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".package-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
