package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	defaultAPIBase      = "https://api.github.com/repos/neptaco/uniforge"
	defaultDownloadBase = "https://github.com/neptaco/uniforge/releases/download"
)

var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

type Options struct {
	CurrentVersion string
	Version        string
	CheckOnly      bool
	Executable     string
	GOOS           string
	GOARCH         string
	APIBase        string
	DownloadBase   string
	HTTPClient     *http.Client
	ValidateBinary func(path, targetVersion string) error
}

type Result struct {
	CurrentVersion string
	TargetVersion  string
	Updated        bool
}

type release struct {
	TagName string `json:"tag_name"`
}

func Run(ctx context.Context, opts Options) (Result, error) {
	opts = defaults(opts)
	if opts.CurrentVersion == "" || opts.CurrentVersion == "dev" || strings.Contains(opts.CurrentVersion, "dirty") {
		return Result{}, fmt.Errorf("self-update is unavailable for development builds (%q)", opts.CurrentVersion)
	}
	if isPackageManagerPath(opts.Executable) {
		return Result{}, fmt.Errorf("%s is managed by a package manager; uninstall it before switching to the standalone installer", opts.Executable)
	}

	target, err := resolveVersion(ctx, opts)
	if err != nil {
		return Result{}, err
	}
	result := Result{CurrentVersion: opts.CurrentVersion, TargetVersion: target}
	if sameVersion(opts.CurrentVersion, target) || opts.CheckOnly {
		return result, nil
	}

	archiveName, binaryName, err := assetNames(opts.GOOS, opts.GOARCH)
	if err != nil {
		return Result{}, err
	}
	base := strings.TrimRight(opts.DownloadBase, "/") + "/" + target
	archiveData, err := download(ctx, opts.HTTPClient, base+"/"+archiveName)
	if err != nil {
		return Result{}, fmt.Errorf("download %s: %w", archiveName, err)
	}
	checksums, err := download(ctx, opts.HTTPClient, base+"/checksums.txt")
	if err != nil {
		return Result{}, fmt.Errorf("download checksums.txt: %w", err)
	}
	if err := verifyChecksum(archiveName, archiveData, string(checksums)); err != nil {
		return Result{}, err
	}
	binary, err := extractBinary(archiveData, binaryName)
	if err != nil {
		return Result{}, err
	}
	if err := replaceExecutable(opts.Executable, binary, target, opts.ValidateBinary); err != nil {
		return Result{}, err
	}
	result.Updated = true
	return result, nil
}

func defaults(opts Options) Options {
	if opts.Version == "" {
		opts.Version = "latest"
	}
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.GOARCH == "" {
		opts.GOARCH = runtime.GOARCH
	}
	if opts.APIBase == "" {
		opts.APIBase = defaultAPIBase
	}
	if opts.DownloadBase == "" {
		opts.DownloadBase = defaultDownloadBase
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	if opts.Executable == "" {
		opts.Executable, _ = os.Executable()
	}
	if resolved, err := filepath.EvalSymlinks(opts.Executable); err == nil {
		opts.Executable = resolved
	}
	if opts.ValidateBinary == nil {
		opts.ValidateBinary = validateBinary
	}
	return opts
}

func resolveVersion(ctx context.Context, opts Options) (string, error) {
	if opts.Version != "latest" {
		if !validVersion(opts.Version) {
			return "", fmt.Errorf("invalid version %q; expected vX.Y.Z", opts.Version)
		}
		return opts.Version, nil
	}
	body, err := download(ctx, opts.HTTPClient, strings.TrimRight(opts.APIBase, "/")+"/releases/latest")
	if err != nil {
		return "", fmt.Errorf("resolve latest release: %w", err)
	}
	var value release
	if err := json.Unmarshal(body, &value); err != nil {
		return "", fmt.Errorf("decode latest release: %w", err)
	}
	if !validVersion(value.TagName) {
		return "", fmt.Errorf("latest release returned invalid version %q", value.TagName)
	}
	return value.TagName, nil
}

func validVersion(value string) bool {
	return versionPattern.MatchString(value)
}

func sameVersion(current, target string) bool {
	return strings.TrimPrefix(current, "v") == strings.TrimPrefix(target, "v")
}

func assetNames(goos, goarch string) (string, string, error) {
	if goarch != "amd64" && goarch != "arm64" {
		return "", "", fmt.Errorf("unsupported architecture: %s", goarch)
	}
	if goos == "windows" && goarch == "arm64" {
		return "", "", errors.New("windows/arm64 is not supported")
	}
	if goos != "darwin" && goos != "linux" && goos != "windows" {
		return "", "", fmt.Errorf("unsupported operating system: %s", goos)
	}
	binary := "uniforge"
	if goos == "windows" {
		binary += ".exe"
	}
	return fmt.Sprintf("uniforge_%s_%s.tar.gz", goos, goarch), binary, nil
}

func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "uniforge-updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 128<<20))
}

func verifyChecksum(name string, data []byte, checksums string) error {
	var expected string
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("checksum not found for %s", name)
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s", name)
	}
	return nil
}

func extractBinary(data []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open release archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tarReader := tar.NewReader(gz)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read release archive: %w", err)
		}
		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == binaryName {
			return io.ReadAll(io.LimitReader(tarReader, 128<<20))
		}
	}
	return nil, fmt.Errorf("%s not found in release archive", binaryName)
}

func replaceExecutable(path string, data []byte, targetVersion string, validate func(string, string) error) error {
	if path == "" {
		return errors.New("cannot determine current executable path")
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".uniforge-update-*")
	if err != nil {
		return fmt.Errorf("create update file: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write update file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync update file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close update file: %w", err)
	}
	if err := os.Chmod(tempPath, 0o755); err != nil {
		return fmt.Errorf("make update executable: %w", err)
	}
	if err := validate(tempPath, targetVersion); err != nil {
		return fmt.Errorf("validate downloaded executable: %w", err)
	}

	backup := path + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("back up current executable: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Rename(backup, path)
		return fmt.Errorf("install update: %w", err)
	}
	if err := os.Remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Windows may keep the running executable open. It is safe to leave the
		// verified backup for removal by a later update.
		return nil
	}
	return nil
}

func validateBinary(path, targetVersion string) error {
	output, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("run --version: %w", err)
	}
	if !sameVersion(strings.TrimSpace(string(output)), targetVersion) {
		return fmt.Errorf("downloaded executable reported %q, expected %s", strings.TrimSpace(string(output)), targetVersion)
	}
	return nil
}

func isPackageManagerPath(path string) bool {
	normalized := strings.ToLower(filepath.ToSlash(path))
	markers := []string{"/cellar/", "/homebrew/", "/scoop/", "/nix/store/", "/chocolatey/"}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
