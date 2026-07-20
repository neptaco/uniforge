package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	autoStateSchemaVersion = 1

	// UnityPackageUpdateCacheFilename is the cache file used for Unity package
	// release checks.
	UnityPackageUpdateCacheFilename = "unity-package-update-check.json"
	defaultUnityPackageAPIBase      = "https://api.github.com/repos/neptaco/uniforge-unity"
)

var errAutoCheckLocked = errors.New("automatic update check is already claimed")

// AutoCheckOptions configures the cached, automatic update check.
type AutoCheckOptions struct {
	CurrentVersion   string
	CachePath        string
	CheckInterval    time.Duration
	ReminderInterval time.Duration
	RetryInterval    time.Duration
	Now              func() time.Time
	APIBase          string
	HTTPClient       *http.Client
}

// AutoCheckDecision describes work that can be done without waiting for the
// network. CheckDue is true only for the process that claimed the next check.
type AutoCheckDecision struct {
	CheckDue      bool
	LatestVersion string
	Notice        *AutoUpdateNotice
}

// AutoUpdateNotice is a cached update notification ready to be displayed.
type AutoUpdateNotice struct {
	CurrentVersion string
	LatestVersion  string
}

type autoCheckState struct {
	SchemaVersion   int       `json:"schema_version"`
	CheckedAt       time.Time `json:"checked_at,omitempty"`
	LastAttemptAt   time.Time `json:"last_attempt_at,omitempty"`
	NextAttemptAt   time.Time `json:"next_attempt_at,omitempty"`
	LatestVersion   string    `json:"latest_version,omitempty"`
	NotifiedVersion string    `json:"notified_version,omitempty"`
	LastNotifiedAt  time.Time `json:"last_notified_at,omitempty"`
	FailureCount    int       `json:"failure_count,omitempty"`
}

type autoCheckTarget struct {
	defaultAPIBase        string
	requireCurrentVersion bool
	includeNotice         bool
	stripVersionPrefix    bool
}

var (
	cliAutoCheckTarget = autoCheckTarget{
		defaultAPIBase:        defaultAPIBase,
		requireCurrentVersion: true,
		includeNotice:         true,
	}
	unityPackageAutoCheckTarget = autoCheckTarget{
		defaultAPIBase:     defaultUnityPackageAPIBase,
		stripVersionPrefix: true,
	}
)

// PrepareAutoCheck reads cached update state and atomically claims a refresh
// when the cache is stale. It never performs network I/O.
func PrepareAutoCheck(opts AutoCheckOptions) (AutoCheckDecision, error) {
	return prepareAutoCheck(opts, cliAutoCheckTarget)
}

// PrepareUnityPackageAutoCheck reads the cached Unity package release and
// atomically claims a refresh when it is due. It never performs network I/O.
func PrepareUnityPackageAutoCheck(opts AutoCheckOptions) (AutoCheckDecision, error) {
	return prepareAutoCheck(opts, unityPackageAutoCheckTarget)
}

func prepareAutoCheck(opts AutoCheckOptions, target autoCheckTarget) (AutoCheckDecision, error) {
	opts = defaultAutoCheckOptions(opts)
	if !target.enabled(opts) {
		return AutoCheckDecision{}, nil
	}

	now := opts.Now()
	state, _ := loadAutoCheckState(opts.CachePath)
	decision := AutoCheckDecision{LatestVersion: target.cachedVersion(state.LatestVersion)}
	if target.includeNotice {
		decision.Notice = cachedNotice(state, opts, now)
	}
	if !state.NextAttemptAt.IsZero() && now.Before(state.NextAttemptAt) {
		return decision, nil
	}

	err := withAutoCheckLock(opts.CachePath, func() error {
		latestState, _ := loadAutoCheckState(opts.CachePath)
		if !latestState.NextAttemptAt.IsZero() && now.Before(latestState.NextAttemptAt) {
			return errAutoCheckLocked
		}
		latestState.SchemaVersion = autoStateSchemaVersion
		latestState.LastAttemptAt = now
		latestState.NextAttemptAt = now.Add(opts.RetryInterval)
		if err := saveAutoCheckState(opts.CachePath, latestState); err != nil {
			return err
		}
		decision.CheckDue = true
		return nil
	})
	if errors.Is(err, errAutoCheckLocked) {
		return decision, nil
	}
	return decision, err
}

// RefreshAutoCheck fetches the latest release and stores it for a later CLI
// invocation. Callers should run this in the detached helper process.
func RefreshAutoCheck(ctx context.Context, opts AutoCheckOptions) error {
	return refreshAutoCheck(ctx, opts, cliAutoCheckTarget)
}

// RefreshUnityPackageAutoCheck fetches the latest uniforge-unity release and
// stores its version without a leading v for a later, network-free read.
func RefreshUnityPackageAutoCheck(ctx context.Context, opts AutoCheckOptions) error {
	return refreshAutoCheck(ctx, opts, unityPackageAutoCheckTarget)
}

func refreshAutoCheck(ctx context.Context, opts AutoCheckOptions, checkTarget autoCheckTarget) error {
	opts = defaultAutoCheckOptions(opts)
	if !checkTarget.enabled(opts) {
		return nil
	}

	apiBase := opts.APIBase
	if apiBase == "" {
		apiBase = checkTarget.defaultAPIBase
	}
	updaterOpts := defaults(Options{
		CurrentVersion: opts.CurrentVersion,
		Version:        "latest",
		APIBase:        apiBase,
	})
	if opts.HTTPClient != nil {
		updaterOpts.HTTPClient = opts.HTTPClient
	}
	latestVersion, fetchErr := resolveVersion(ctx, updaterOpts)
	if fetchErr == nil {
		latestVersion = checkTarget.cachedVersion(latestVersion)
	}
	now := opts.Now()

	err := withAutoCheckLock(opts.CachePath, func() error {
		state, _ := loadAutoCheckState(opts.CachePath)
		state.SchemaVersion = autoStateSchemaVersion
		state.LastAttemptAt = now
		if fetchErr != nil {
			state.FailureCount++
			state.NextAttemptAt = now.Add(retryDelay(opts.RetryInterval, state.FailureCount))
		} else {
			state.CheckedAt = now
			state.NextAttemptAt = now.Add(opts.CheckInterval)
			state.LatestVersion = latestVersion
			state.FailureCount = 0
		}
		return saveAutoCheckState(opts.CachePath, state)
	})
	if err != nil && !errors.Is(err, errAutoCheckLocked) {
		return err
	}
	return fetchErr
}

// ReadUnityPackageLatestVersion reads the cached Unity package release without
// claiming or refreshing it. Invalid cached versions are treated as unknown.
func ReadUnityPackageLatestVersion(cachePath string) (string, error) {
	if cachePath == "" {
		return "", nil
	}
	state, err := loadAutoCheckState(cachePath)
	if err != nil {
		return "", err
	}
	return unityPackageAutoCheckTarget.cachedVersion(state.LatestVersion), nil
}

// RecordAutoNotification throttles repeated notifications for one release.
func RecordAutoNotification(opts AutoCheckOptions, latestVersion string) error {
	opts = defaultAutoCheckOptions(opts)
	if opts.CachePath == "" {
		return nil
	}
	now := opts.Now()
	return withAutoCheckLock(opts.CachePath, func() error {
		state, _ := loadAutoCheckState(opts.CachePath)
		state.SchemaVersion = autoStateSchemaVersion
		state.NotifiedVersion = latestVersion
		state.LastNotifiedAt = now
		return saveAutoCheckState(opts.CachePath, state)
	})
}

func defaultAutoCheckOptions(opts AutoCheckOptions) AutoCheckOptions {
	if opts.CheckInterval <= 0 {
		opts.CheckInterval = 24 * time.Hour
	}
	if opts.ReminderInterval <= 0 {
		opts.ReminderInterval = 7 * 24 * time.Hour
	}
	if opts.RetryInterval <= 0 {
		opts.RetryInterval = time.Hour
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return opts
}

func (target autoCheckTarget) enabled(opts AutoCheckOptions) bool {
	if opts.CachePath == "" {
		return false
	}
	return !target.requireCurrentVersion || isReleaseVersion(opts.CurrentVersion)
}

func (target autoCheckTarget) cachedVersion(version string) string {
	version = strings.TrimSpace(version)
	if _, ok := parseVersion(version); !ok {
		return ""
	}
	if target.stripVersionPrefix {
		return strings.TrimPrefix(version, "v")
	}
	return version
}

func cachedNotice(state autoCheckState, opts AutoCheckOptions, now time.Time) *AutoUpdateNotice {
	if !isNewerVersion(state.LatestVersion, opts.CurrentVersion) {
		return nil
	}
	if state.NotifiedVersion == state.LatestVersion &&
		!state.LastNotifiedAt.IsZero() &&
		now.Before(state.LastNotifiedAt.Add(opts.ReminderInterval)) {
		return nil
	}
	return &AutoUpdateNotice{
		CurrentVersion: normalizeVersion(opts.CurrentVersion),
		LatestVersion:  normalizeVersion(state.LatestVersion),
	}
}

func retryDelay(base time.Duration, failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	delay := base
	for i := 1; i < failures && delay < 24*time.Hour; i++ {
		delay *= 2
	}
	if delay > 24*time.Hour {
		return 24 * time.Hour
	}
	return delay
}

func isReleaseVersion(version string) bool {
	version = normalizeVersion(version)
	return validVersion(version)
}

// IsNewerVersion reports whether candidate is a newer X.Y.Z release than
// current. Both raw and v-prefixed versions are accepted.
func IsNewerVersion(candidate, current string) bool {
	candidateParts, ok := parseVersion(candidate)
	if !ok {
		return false
	}
	currentParts, ok := parseVersion(current)
	if !ok {
		return false
	}
	for i := range candidateParts {
		if candidateParts[i] != currentParts[i] {
			return candidateParts[i] > currentParts[i]
		}
	}
	return false
}

func isNewerVersion(candidate, current string) bool {
	return IsNewerVersion(candidate, current)
}

func parseVersion(version string) ([3]int, bool) {
	var result [3]int
	parts := strings.Split(strings.TrimPrefix(normalizeVersion(version), "v"), ".")
	if len(parts) != len(result) {
		return result, false
	}
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return result, false
		}
		result[i] = value
	}
	return result, true
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	if version != "" && !strings.HasPrefix(version, "v") {
		return "v" + version
	}
	return version
}

func loadAutoCheckState(path string) (autoCheckState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return autoCheckState{}, nil
	}
	if err != nil {
		return autoCheckState{}, err
	}
	var state autoCheckState
	if err := json.Unmarshal(data, &state); err != nil {
		return autoCheckState{}, fmt.Errorf("decode automatic update cache: %w", err)
	}
	if state.SchemaVersion != 0 && state.SchemaVersion != autoStateSchemaVersion {
		return autoCheckState{}, fmt.Errorf("unsupported automatic update cache schema %d", state.SchemaVersion)
	}
	return state, nil
}

func saveAutoCheckState(path string, state autoCheckState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create automatic update cache directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".update-check-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace automatic update cache: %w", err)
	}
	return nil
}

func withAutoCheckLock(cachePath string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return err
	}
	lockPath := cachePath + ".lock"
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > 10*time.Minute {
			_ = os.Remove(lockPath)
			lock, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		}
	}
	if errors.Is(err, os.ErrExist) {
		return errAutoCheckLocked
	}
	if err != nil {
		return err
	}
	_ = lock.Close()
	defer func() { _ = os.Remove(lockPath) }()
	return fn()
}
