package hub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neptaco/uniforge/pkg/ui"
)

// UnityRelease represents a Unity release with its metadata
type UnityRelease struct {
	Version         string
	Changeset       string
	LTS             bool
	Stream          string // "LTS", "TECH", "BETA", "SUPPORTED"
	Modules         []ModuleInfo
	Installed       bool
	InstalledPath   string
	Architecture    string // "x86_64" or "arm64"
	ReleaseDate     time.Time
	Recommended     bool
	ReleaseNotesURL string
	DownloadSize    int64  // bytes
	InstalledSize   int64  // bytes
	SecurityAlert   string // Security alert message if any
}

// ModuleInfo represents a module available for a Unity version
type ModuleInfo struct {
	ID            string
	Name          string
	Description   string
	Category      string // "PLATFORM", "DEV_TOOL", "LANGUAGE_PACK", "DOCUMENTATION"
	Installed     bool
	Hidden        bool
	DownloadSize  int64 // bytes
	InstalledSize int64 // bytes
}

// IsVisible returns true if the module should be shown in UI
func (m ModuleInfo) IsVisible() bool {
	return !m.Hidden && m.Category == "PLATFORM"
}

// VersionStream represents a major.minor version stream (e.g., "2022.3 LTS")
type VersionStream struct {
	MajorMinor    string // e.g., "2022.3"
	DisplayName   string // e.g., "2022.3 LTS"
	TotalCount    int
	LatestVersion string
	LTS           bool
	IsUnity6      bool
}

// releasesFileData represents the structure of releases.json
type releasesFileData struct {
	Official []releasesFileEntry `json:"official"`
	Beta     []releasesFileEntry `json:"beta"`
}

type releasesFileEntry struct {
	Version      string                   `json:"version"`
	LTS          bool                     `json:"lts"`
	DownloadURL  string                   `json:"downloadUrl"`
	Arch         string                   `json:"arch"`
	Modules      []releasesFileModuleInfo `json:"modules"`
	DownloadSize int64                    `json:"downloadSize"`
}

type releasesFileModuleInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Visible     bool   `json:"visible"`
}

const (
	releaseAPIPageLimit   = 25 // maximum limit accepted by the API
	releaseAPIMinInterval = 150 * time.Millisecond
	releaseAPIMaxAttempts = 3
	// releaseAPIMaxPages bounds pagination per version as a safety valve in
	// case the API reports an inconsistent total that is never reached
	releaseAPIMaxPages = 100
)

// releaseAPIClient wraps access to the Unity Release API (REST):
// https://services.docs.unity.com/release/v1/
type releaseAPIClient struct {
	baseURL     string
	minInterval time.Duration
	retryDelay  time.Duration
	httpClient  *http.Client

	mu          sync.Mutex
	lastRequest time.Time
}

// defaultReleaseAPI is the shared instance for all Release API access. The
// rate limit (10 req/s and 1000 req/30min, see the unity-ratelimit response
// header) applies per IP, so every caller in the process must go through the
// same limiter.
var defaultReleaseAPI = &releaseAPIClient{
	baseURL:     "https://services.api.unity.com/unity/editor/release/v1/releases",
	minInterval: releaseAPIMinInterval,
	retryDelay:  2 * time.Second,
	httpClient:  &http.Client{Timeout: 30 * time.Second},
}

// releaseAPIResponse is a page of results from the Unity Release API
type releaseAPIResponse struct {
	Total   int                 `json:"total"`
	Results []releaseAPIRelease `json:"results"`
}

// releaseAPIRelease is a single release entry. Unlike the retired GraphQL
// API, there is no security-label field, so UnityRelease.SecurityAlert
// cannot be populated from this API.
type releaseAPIRelease struct {
	Version       string                 `json:"version"`
	ShortRevision string                 `json:"shortRevision"`
	Stream        string                 `json:"stream"`
	ReleaseDate   time.Time              `json:"releaseDate"`
	Recommended   bool                   `json:"recommended"`
	ReleaseNotes  releaseAPIReleaseNotes `json:"releaseNotes"`
	Downloads     []releaseAPIDownload   `json:"downloads"`
}

type releaseAPIReleaseNotes struct {
	URL string `json:"url"`
}

// digitalValue is a size value; the API always reports unit BYTE
type digitalValue struct {
	Value float64 `json:"value"`
}

type releaseAPIDownload struct {
	Platform      string             `json:"platform"`
	Architecture  string             `json:"architecture"`
	DownloadSize  digitalValue       `json:"downloadSize"`
	InstalledSize digitalValue       `json:"installedSize"`
	Modules       []releaseAPIModule `json:"modules"`
}

type releaseAPIModule struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Category      string       `json:"category"`
	Hidden        bool         `json:"hidden"`
	DownloadSize  digitalValue `json:"downloadSize"`
	InstalledSize digitalValue `json:"installedSize"`
}

// waitTurn spaces out requests so that all callers combined stay under the
// API rate limit
func (a *releaseAPIClient) waitTurn() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if wait := a.minInterval - time.Since(a.lastRequest); wait > 0 {
		time.Sleep(wait)
	}
	a.lastRequest = time.Now()
}

// fetchPage performs a single Unity Release API request with rate limiting
// and retry on HTTP 429
func (a *releaseAPIClient) fetchPage(params url.Values) (*releaseAPIResponse, error) {
	reqURL := a.baseURL + "?" + params.Encode()

	for attempt := 1; ; attempt++ {
		a.waitTurn()

		resp, err := a.httpClient.Get(reqURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch from Unity Release API: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < releaseAPIMaxAttempts {
			delay := a.retryDelay
			if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
				delay = time.Duration(secs) * time.Second
			}
			ui.Debug("Unity Release API rate limited, retrying", "attempt", attempt, "delay", delay)
			time.Sleep(delay)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %d from Unity Release API: %s", resp.StatusCode, truncateBody(body))
		}

		var page releaseAPIResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return &page, nil
	}
}

// forEachParallel runs fn for every item concurrently and waits for all to
// finish. Request pacing is handled globally by releaseAPIClient.waitTurn,
// so no additional concurrency bound is needed here.
func forEachParallel[T any](items []T, fn func(T)) {
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Go(func() { fn(item) })
	}
	wg.Wait()
}

// truncateBody shortens an error response body for inclusion in error messages
func truncateBody(body []byte) string {
	const maxLen = 200
	if len(body) > maxLen {
		return string(body[:maxLen]) + "..."
	}
	return string(body)
}

// releasesCacheData represents the cached release data
type releasesCacheData struct {
	Streams   map[string]streamCacheEntry `json:"streams"`
	Releases  []releaseCacheEntry         `json:"releases"`
	UpdatedAt time.Time                   `json:"updatedAt"`
}

type streamCacheEntry struct {
	TotalCount    int    `json:"totalCount"`
	LatestVersion string `json:"latestVersion"`
	LTS           bool   `json:"lts"`
}

type releaseCacheEntry struct {
	Version         string             `json:"version"`
	Changeset       string             `json:"changeset"`
	LTS             bool               `json:"lts"`
	Stream          string             `json:"stream"`
	ReleaseDate     time.Time          `json:"releaseDate,omitempty"`
	Recommended     bool               `json:"recommended,omitempty"`
	ReleaseNotesURL string             `json:"releaseNotesUrl,omitempty"`
	DownloadSize    int64              `json:"downloadSize,omitempty"`
	InstalledSize   int64              `json:"installedSize,omitempty"`
	Modules         []moduleCacheEntry `json:"modules,omitempty"`
}

type moduleCacheEntry struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Category      string `json:"category"`
	Hidden        bool   `json:"hidden,omitempty"`
	DownloadSize  int64  `json:"downloadSize,omitempty"`
	InstalledSize int64  `json:"installedSize,omitempty"`
}

// baseMajorVersions is the baseline list of major versions (fallback)
var baseMajorVersions = []string{
	"6000.0", "6000.1", "6000.2", "6000.3", // Unity 6
	"2023.1", "2023.2", "2023.3",
	"2022.1", "2022.2", "2022.3",
	"2021.1", "2021.2", "2021.3",
	"2020.1", "2020.2", "2020.3",
	"2019.4",
}

// DiscoverMajorVersions discovers all major versions from multiple sources.
// The result is memoized per Client: callers like `editor available` and the
// TUI trigger discovery several times per command, and each discovery costs
// one API request per release stream.
func (c *Client) DiscoverMajorVersions() []string {
	c.majorVersionsOnce.Do(func() {
		c.majorVersions = c.discoverMajorVersions()
	})
	return c.majorVersions
}

func (c *Client) discoverMajorVersions() []string {
	seen := make(map[string]bool)

	// 1. Fetch recent major versions from the Release API
	if apiVersions, err := c.fetchMajorVersionsFromAPI(); err == nil {
		for _, v := range apiVersions {
			seen[v] = true
		}
	}

	// 2. Always include the baseline: the API discovery only covers the most
	// recent releases per stream, so older majors would be lost without it
	for _, v := range baseMajorVersions {
		seen[v] = true
	}

	// 3. Extract from cache (may have versions not in current API response)
	if !c.NoCache {
		if cache, err := c.LoadCache(); err == nil && cache != nil {
			for _, entry := range cache.Releases {
				mm := GetMajorMinorFromVersion(entry.Version)
				if mm != "" {
					seen[mm] = true
				}
			}
			for mm := range cache.Streams {
				seen[mm] = true
			}
		}
	}

	// 4. Extract from Unity Hub's releases.json
	if releases, err := c.LoadReleasesFromFile(); err == nil {
		for _, r := range releases {
			mm := GetMajorMinorFromVersion(r.Version)
			if mm != "" {
				seen[mm] = true
			}
		}
	}

	// Convert to slice and sort
	var result []string
	for mm := range seen {
		result = append(result, mm)
	}

	sort.Slice(result, func(i, j int) bool {
		return compareVersions(result[i]+".0", result[j]+".0") > 0
	})

	return result
}

// releaseAPIStreams is every stream the Release API can filter by
var releaseAPIStreams = []string{"LTS", "SUPPORTED", "TECH", "BETA", "ALPHA"}

// fetchMajorVersionsFromAPI discovers major versions from the latest page of
// each release stream. Older majors that no longer receive releases are
// covered by baseMajorVersions in DiscoverMajorVersions.
func (c *Client) fetchMajorVersionsFromAPI() ([]string, error) {
	// Only the version strings are needed; the platform filter just trims
	// the payload (downloads of other platforms are omitted)
	platform, _ := c.detectPlatformArch()

	seen := make(map[string]bool)
	var mu sync.Mutex
	var firstErr error

	forEachParallel(releaseAPIStreams, func(stream string) {
		params := url.Values{}
		params.Set("stream", stream)
		params.Set("limit", strconv.Itoa(releaseAPIPageLimit))
		params.Set("order", "RELEASE_DATE_DESC")
		params.Set("platform", platform)

		page, err := defaultReleaseAPI.fetchPage(params)

		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			ui.Debug("Failed to fetch major versions", "stream", stream, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		for _, rel := range page.Results {
			if mm := GetMajorMinorFromVersion(rel.Version); mm != "" {
				seen[mm] = true
			}
		}
	})

	if len(seen) == 0 && firstErr != nil {
		return nil, firstErr
	}

	var versions []string
	for v := range seen {
		versions = append(versions, v)
	}

	return versions, nil
}

// GetReleasesFilePath returns the path to Unity Hub's releases.json
func (c *Client) GetReleasesFilePath() string {
	var basePath string

	switch runtime.GOOS {
	case "darwin":
		basePath = filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "UnityHub")
	case "windows":
		basePath = filepath.Join(os.Getenv("APPDATA"), "UnityHub")
	case "linux":
		basePath = filepath.Join(os.Getenv("HOME"), ".config", "UnityHub")
	default:
		return ""
	}

	return filepath.Join(basePath, "releases.json")
}

// getCacheFilePath returns the path to uniforge's release cache
func (c *Client) getReleaseCacheFilePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	return filepath.Join(cacheDir, "uniforge", "releases-cache.json")
}

// LoadReleasesFromFile loads releases from Unity Hub's releases.json
func (c *Client) LoadReleasesFromFile() ([]UnityRelease, error) {
	releasesFilePath := c.GetReleasesFilePath()
	if releasesFilePath == "" {
		return nil, fmt.Errorf("could not determine releases file path")
	}

	data, err := os.ReadFile(releasesFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []UnityRelease{}, nil
		}
		return nil, fmt.Errorf("failed to read releases file: %w", err)
	}

	var releasesData releasesFileData
	if err := json.Unmarshal(data, &releasesData); err != nil {
		return nil, fmt.Errorf("failed to parse releases file: %w", err)
	}

	var result []UnityRelease

	// Process official releases
	for _, entry := range releasesData.Official {
		release := c.convertFileEntryToRelease(entry)
		release.Stream = "TECH"
		if entry.LTS {
			release.Stream = "LTS"
		}
		result = append(result, release)
	}

	// Process beta releases
	for _, entry := range releasesData.Beta {
		release := c.convertFileEntryToRelease(entry)
		release.Stream = "BETA"
		result = append(result, release)
	}

	return result, nil
}

// convertFileEntryToRelease converts a releases.json entry to UnityRelease
func (c *Client) convertFileEntryToRelease(entry releasesFileEntry) UnityRelease {
	release := UnityRelease{
		Version:      entry.Version,
		LTS:          entry.LTS,
		Architecture: entry.Arch,
	}

	// Extract changeset from download URL
	// Format: https://download.unity3d.com/download_unity/ffc62b691db5/...
	if entry.DownloadURL != "" {
		parts := strings.Split(entry.DownloadURL, "/")
		for i, part := range parts {
			if part == "download_unity" && i+1 < len(parts) {
				release.Changeset = parts[i+1]
				break
			}
		}
	}

	// Convert modules
	for _, mod := range entry.Modules {
		release.Modules = append(release.Modules, ModuleInfo{
			ID:          mod.ID,
			Name:        mod.Name,
			Description: mod.Description,
			Category:    mod.Category,
			Hidden:      !mod.Visible,
		})
	}

	return release
}

// FetchStreams fetches stream metadata (totalCount, latestVersion) from the Release API
func (c *Client) FetchStreams() ([]VersionStream, error) {
	majorVersions := c.DiscoverMajorVersions()

	var streams []VersionStream
	var mu sync.Mutex
	var firstErr error

	// Per-version failures are tolerated (the stream is just missing from
	// the listing), but a complete failure must surface as an error so
	// callers do not mistake an API outage for an empty stream list
	forEachParallel(majorVersions, func(mm string) {
		stream, err := c.fetchStreamMetadata(mm)

		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			ui.Debug("Failed to fetch stream metadata", "version", mm, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			return
		}

		if stream.TotalCount > 0 {
			streams = append(streams, stream)
		}
	})

	if len(streams) == 0 && firstErr != nil {
		return nil, firstErr
	}

	// Sort streams by version (newest first)
	sort.Slice(streams, func(i, j int) bool {
		return compareVersions(streams[i].MajorMinor+".0", streams[j].MajorMinor+".0") > 0
	})

	return streams, nil
}

// fetchStreamMetadata fetches metadata for a single stream
func (c *Client) fetchStreamMetadata(majorMinor string) (VersionStream, error) {
	platform, _ := c.detectPlatformArch()

	params := url.Values{}
	params.Set("version", majorMinor)
	params.Set("limit", "1")
	params.Set("order", "RELEASE_DATE_DESC")
	params.Set("platform", platform)

	page, err := defaultReleaseAPI.fetchPage(params)
	if err != nil {
		return VersionStream{}, err
	}

	latestVersion := ""
	lts := false

	if len(page.Results) > 0 {
		latestVersion = page.Results[0].Version
		lts = page.Results[0].Stream == "LTS"
	}

	return newVersionStream(majorMinor, page.Total, latestVersion, lts), nil
}

func newVersionStream(majorMinor string, totalCount int, latestVersion string, lts bool) VersionStream {
	stream := VersionStream{
		MajorMinor:    majorMinor,
		DisplayName:   majorMinor,
		TotalCount:    totalCount,
		LatestVersion: latestVersion,
		LTS:           lts,
		IsUnity6:      strings.HasPrefix(majorMinor, "6000"),
	}

	if stream.IsUnity6 {
		stream.DisplayName = fmt.Sprintf("Unity 6 (%s)", majorMinor)
	}
	if stream.LTS {
		stream.DisplayName += " LTS"
	}
	return stream
}

// FetchReleasesForStream fetches all releases for a specific stream
func (c *Client) FetchReleasesForStream(majorMinor string) ([]UnityRelease, error) {
	return c.FetchReleases([]string{majorMinor})
}

// FetchReleases fetches all releases for the given major.minor versions from
// the Unity Release API. Failures for individual versions are tolerated as
// long as at least one version succeeds.
func (c *Client) FetchReleases(majorMinorVersions []string) ([]UnityRelease, error) {
	releases, _, err := c.fetchReleasesAndTotals(majorMinorVersions)
	return releases, err
}

func (c *Client) fetchReleasesAndTotals(majorMinorVersions []string) ([]UnityRelease, map[string]int, error) {
	if len(majorMinorVersions) == 0 {
		return nil, nil, nil
	}

	platform, arch := c.detectPlatformArch()

	var allReleases []UnityRelease
	totals := make(map[string]int, len(majorMinorVersions))
	var mu sync.Mutex
	var firstErr error

	forEachParallel(majorMinorVersions, func(mm string) {
		releases, total, err := c.fetchReleasesForVersion(mm, platform, arch)

		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			ui.Debug("Failed to fetch releases", "version", mm, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		totals[mm] = total
		allReleases = append(allReleases, releases...)
	})

	if len(allReleases) == 0 && firstErr != nil {
		return nil, nil, firstErr
	}

	return allReleases, totals, nil
}

// fetchReleasesForVersion pages through all releases of one major.minor version
func (c *Client) fetchReleasesForVersion(majorMinor, platform, arch string) ([]UnityRelease, int, error) {
	var releases []UnityRelease
	totalCount := 0
	for offset, pages := 0, 0; ; pages++ {
		if pages >= releaseAPIMaxPages {
			ui.Debug("Stopping pagination at page limit", "version", majorMinor, "pages", pages)
			break
		}

		params := url.Values{}
		params.Set("version", majorMinor)
		params.Set("limit", strconv.Itoa(releaseAPIPageLimit))
		params.Set("offset", strconv.Itoa(offset))
		params.Set("order", "RELEASE_DATE_DESC")
		// Filter by platform only: adding architecture would drop releases
		// that lack a build for the current architecture (e.g. pre-ARM64
		// macOS versions), which should still be listed
		params.Set("platform", platform)

		page, err := defaultReleaseAPI.fetchPage(params)
		if err != nil {
			return nil, totalCount, err
		}
		if pages == 0 {
			totalCount = page.Total
		}

		for _, rel := range page.Results {
			releases = append(releases, c.convertAPIRelease(rel, platform, arch))
		}

		offset += len(page.Results)
		if len(page.Results) == 0 || offset >= page.Total {
			break
		}
	}

	return releases, totalCount, nil
}

// BuildStreamsFromReleases builds stream metadata from already fetched release data.
func BuildStreamsFromReleases(releases []UnityRelease, totals map[string]int) []VersionStream {
	if len(releases) == 0 {
		return nil
	}

	grouped := make(map[string][]UnityRelease)
	for _, release := range releases {
		majorMinor := GetMajorMinorFromVersion(release.Version)
		grouped[majorMinor] = append(grouped[majorMinor], release)
	}

	streams := make([]VersionStream, 0, len(grouped))
	for majorMinor, group := range grouped {
		if len(group) == 0 {
			continue
		}

		latest := group[0]
		for _, release := range group[1:] {
			if compareVersions(release.Version, latest.Version) > 0 {
				latest = release
			}
		}

		totalCount := len(group)
		if totals != nil && totals[majorMinor] > 0 {
			totalCount = totals[majorMinor]
		} else {
			// Happens when the API fetch failed for this version; the count
			// may be lower than the real total and invalidate the cache on
			// the next run
			ui.Debug("No API total for stream, using fetched release count",
				"stream", majorMinor, "count", totalCount)
		}

		streams = append(streams, newVersionStream(majorMinor, totalCount, latest.Version, latest.Stream == "LTS"))
	}

	sort.Slice(streams, func(i, j int) bool {
		return compareVersions(streams[i].MajorMinor+".0", streams[j].MajorMinor+".0") > 0
	})

	return streams
}

// convertAPIRelease converts a Release API entry to UnityRelease
func (c *Client) convertAPIRelease(rel releaseAPIRelease, platform, arch string) UnityRelease {
	release := UnityRelease{
		Version:         rel.Version,
		Changeset:       rel.ShortRevision,
		Stream:          rel.Stream,
		LTS:             rel.Stream == "LTS",
		ReleaseDate:     rel.ReleaseDate,
		Recommended:     rel.Recommended,
		ReleaseNotesURL: rel.ReleaseNotes.URL,
	}

	for _, dl := range rel.Downloads {
		if dl.Platform == platform && dl.Architecture == arch {
			release.DownloadSize = int64(dl.DownloadSize.Value)
			release.InstalledSize = int64(dl.InstalledSize.Value)

			for _, mod := range dl.Modules {
				release.Modules = append(release.Modules, ModuleInfo{
					ID:            mod.ID,
					Name:          mod.Name,
					Description:   mod.Description,
					Category:      mod.Category,
					Hidden:        mod.Hidden,
					DownloadSize:  int64(mod.DownloadSize.Value),
					InstalledSize: int64(mod.InstalledSize.Value),
				})
			}
			break
		}
	}

	return release
}

// detectPlatformArch returns the current platform and architecture in the
// Release API's enum format
func (c *Client) detectPlatformArch() (platform, arch string) {
	switch runtime.GOOS {
	case "darwin":
		platform = "MAC_OS"
	case "windows":
		platform = "WINDOWS"
	case "linux":
		platform = "LINUX"
	default:
		platform = "WINDOWS"
	}

	if runtime.GOARCH == "arm64" {
		arch = "ARM64"
	} else {
		arch = "X86_64"
	}

	return platform, arch
}

// ClearCache removes the cache file
func (c *Client) ClearCache() error {
	cachePath := c.getReleaseCacheFilePath()

	if err := os.Remove(cachePath); err != nil {
		if os.IsNotExist(err) {
			// Cache doesn't exist, nothing to clear
			return nil
		}
		return err
	}
	return nil
}

// LoadCache loads cached releases
func (c *Client) LoadCache() (*releasesCacheData, error) {
	cachePath := c.getReleaseCacheFilePath()

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	var cache releasesCacheData
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	return &cache, nil
}

// SaveCache saves releases to cache
func (c *Client) SaveCache(streams []VersionStream, releases []UnityRelease) error {
	cachePath := c.getReleaseCacheFilePath()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return err
	}

	cache := releasesCacheData{
		Streams:   make(map[string]streamCacheEntry),
		UpdatedAt: time.Now(),
	}

	for _, s := range streams {
		cache.Streams[s.MajorMinor] = streamCacheEntry{
			TotalCount:    s.TotalCount,
			LatestVersion: s.LatestVersion,
			LTS:           s.LTS,
		}
	}

	for _, r := range releases {
		entry := releaseCacheEntry{
			Version:         r.Version,
			Changeset:       r.Changeset,
			LTS:             r.LTS,
			Stream:          r.Stream,
			ReleaseDate:     r.ReleaseDate,
			Recommended:     r.Recommended,
			ReleaseNotesURL: r.ReleaseNotesURL,
			DownloadSize:    r.DownloadSize,
			InstalledSize:   r.InstalledSize,
		}

		// Convert modules
		for _, mod := range r.Modules {
			entry.Modules = append(entry.Modules, moduleCacheEntry{
				ID:            mod.ID,
				Name:          mod.Name,
				Category:      mod.Category,
				Hidden:        mod.Hidden,
				DownloadSize:  mod.DownloadSize,
				InstalledSize: mod.InstalledSize,
			})
		}

		cache.Releases = append(cache.Releases, entry)
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, data, 0644)
}

// CheckCacheValidity checks if cache is valid by comparing totalCount
func (c *Client) CheckCacheValidity(cache *releasesCacheData, currentStreams []VersionStream) bool {
	if cache == nil || len(cache.Streams) == 0 {
		return false
	}

	for _, stream := range currentStreams {
		cached, exists := cache.Streams[stream.MajorMinor]
		if !exists {
			return false
		}
		if cached.TotalCount != stream.TotalCount {
			ui.Debug("Cache invalid: totalCount changed", "stream", stream.MajorMinor,
				"cached", cached.TotalCount, "current", stream.TotalCount)
			return false
		}
	}

	return true
}

// ConvertCacheToReleases converts cached entries to UnityRelease
func (c *Client) ConvertCacheToReleases(cache *releasesCacheData) []UnityRelease {
	var releases []UnityRelease
	for _, entry := range cache.Releases {
		release := UnityRelease{
			Version:         entry.Version,
			Changeset:       entry.Changeset,
			LTS:             entry.LTS,
			Stream:          entry.Stream,
			ReleaseDate:     entry.ReleaseDate,
			Recommended:     entry.Recommended,
			ReleaseNotesURL: entry.ReleaseNotesURL,
			DownloadSize:    entry.DownloadSize,
			InstalledSize:   entry.InstalledSize,
		}

		// Convert modules
		for _, mod := range entry.Modules {
			release.Modules = append(release.Modules, ModuleInfo{
				ID:            mod.ID,
				Name:          mod.Name,
				Category:      mod.Category,
				Hidden:        mod.Hidden,
				DownloadSize:  mod.DownloadSize,
				InstalledSize: mod.InstalledSize,
			})
		}

		releases = append(releases, release)
	}
	return releases
}

// GetAllReleases loads releases from cache or API, enriches with install status
func (c *Client) GetAllReleases() ([]UnityRelease, error) {
	releases, _, err := c.GetAllReleasesWithStreams()
	return releases, err
}

// GetAllReleasesWithStreams loads releases and derives stream metadata from the same API response.
func (c *Client) GetAllReleasesWithStreams() ([]UnityRelease, []VersionStream, error) {
	// Load from releases.json (has module info)
	localReleases, err := c.LoadReleasesFromFile()
	if err != nil {
		ui.Debug("Failed to load releases from file", "error", err)
		localReleases = []UnityRelease{}
	}

	// Fetch from the Release API (has all versions)
	majorVersions := c.DiscoverMajorVersions()
	apiReleases, totals, err := c.fetchReleasesAndTotals(majorVersions)
	if err != nil {
		ui.Debug("Failed to fetch releases from Unity Release API", "error", err)
	}

	// Merge: API releases + local releases (local has module info)
	releases := mergeReleases(apiReleases, localReleases)

	// Deduplicate releases by version
	releases = deduplicateReleases(releases)

	// Enrich with install status
	releases = c.EnrichReleasesWithInstallStatus(releases)

	// Sort by release date (newest first), fallback to version comparison
	sort.Slice(releases, func(i, j int) bool {
		// If both have release dates, sort by date
		if !releases[i].ReleaseDate.IsZero() && !releases[j].ReleaseDate.IsZero() {
			return releases[i].ReleaseDate.After(releases[j].ReleaseDate)
		}
		// Fallback to version comparison
		return compareVersions(releases[i].Version, releases[j].Version) > 0
	})

	streams := BuildStreamsFromReleases(releases, totals)

	return releases, streams, nil
}

// mergeReleases merges API releases with local releases
// API releases have metadata (releaseDate, recommended, etc.)
// Local releases may have additional module info
func mergeReleases(apiReleases, localReleases []UnityRelease) []UnityRelease {
	// Create a map of local releases for quick lookup
	localMap := make(map[string]UnityRelease)
	for _, r := range localReleases {
		localMap[r.Version] = r
	}

	var result []UnityRelease

	// Add API releases, enriching with local module info if available
	for _, apiRelease := range apiReleases {
		if localRelease, exists := localMap[apiRelease.Version]; exists {
			// Use API release (has metadata), but add local modules if API has none
			if len(apiRelease.Modules) == 0 && len(localRelease.Modules) > 0 {
				apiRelease.Modules = localRelease.Modules
			}
			delete(localMap, apiRelease.Version)
		}
		result = append(result, apiRelease)
	}

	// Add remaining local releases that weren't in API
	for _, r := range localReleases {
		if _, exists := localMap[r.Version]; exists {
			result = append(result, r)
		}
	}

	return result
}

// deduplicateReleases removes duplicate versions, keeping the one with more module info
func deduplicateReleases(releases []UnityRelease) []UnityRelease {
	seen := make(map[string]int) // version -> index in result
	var result []UnityRelease

	for _, r := range releases {
		if idx, exists := seen[r.Version]; exists {
			// Keep the one with more modules or changeset
			existing := result[idx]
			if len(r.Modules) > len(existing.Modules) ||
				(r.Changeset != "" && existing.Changeset == "") {
				result[idx] = r
			}
		} else {
			seen[r.Version] = len(result)
			result = append(result, r)
		}
	}

	return result
}

// EnrichReleasesWithInstallStatus adds install status to releases
func (c *Client) EnrichReleasesWithInstallStatus(releases []UnityRelease) []UnityRelease {
	// Get installed editors
	installedEditors, err := c.ListInstalledEditors()
	if err != nil {
		ui.Debug("Failed to list installed editors", "error", err)
		return releases
	}

	// Create a map of installed versions
	installedMap := make(map[string]EditorInfo)
	for _, editor := range installedEditors {
		installedMap[editor.Version] = editor
	}

	// Update releases with install status
	for i := range releases {
		if editor, ok := installedMap[releases[i].Version]; ok {
			releases[i].Installed = true
			releases[i].InstalledPath = editor.Path

			// Enrich modules with install status
			for j := range releases[i].Modules {
				releases[i].Modules[j].Installed = c.IsModuleInstalled(editor.Path, releases[i].Modules[j].ID)
			}
		}
	}

	return releases
}

// GetCommonModules returns a list of commonly used modules
func GetCommonModules() []ModuleInfo {
	return []ModuleInfo{
		{ID: "android", Name: "Android Build Support", Category: "PLATFORM"},
		{ID: "ios", Name: "iOS Build Support", Category: "PLATFORM"},
		{ID: "webgl", Name: "WebGL Build Support", Category: "PLATFORM"},
		{ID: "windows-il2cpp", Name: "Windows Build Support (IL2CPP)", Category: "PLATFORM"},
		{ID: "linux-il2cpp", Name: "Linux Build Support (IL2CPP)", Category: "PLATFORM"},
		{ID: "mac-il2cpp", Name: "Mac Build Support (IL2CPP)", Category: "PLATFORM"},
	}
}

// compareVersions compares two Unity version strings
// Returns: >0 if v1 > v2, <0 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	// Parse version like "2022.3.60f1", "6000.4.0b3", "6000.4.0a5"
	p1 := parseVersionParts(v1)
	p2 := parseVersionParts(v2)

	// Compare each part
	for i := 0; i < len(p1) && i < len(p2); i++ {
		if p1[i] > p2[i] {
			return 1
		}
		if p1[i] < p2[i] {
			return -1
		}
	}

	// If all parts are equal, longer version is greater
	return len(p1) - len(p2)
}

// parseVersionParts parses a version string into comparable integer parts
// Format: major.minor.patch[a|b|f]N -> [major, minor, patch, releaseType, releaseNum]
// releaseType: alpha=1, beta=2, final=3
func parseVersionParts(version string) []int {
	var parts []int

	// Split by dot
	dotParts := strings.Split(version, ".")
	for i, part := range dotParts {
		if i == len(dotParts)-1 {
			// Last part may contain suffix like "60f1", "0b3", "0a5"
			num, releaseType, releaseNum := parseVersionSuffix(part)
			parts = append(parts, num, releaseType, releaseNum)
		} else {
			var num int
			_, _ = fmt.Sscanf(part, "%d", &num)
			parts = append(parts, num)
		}
	}

	return parts
}

// parseVersionSuffix parses "60f1" -> (60, 3, 1), "0b3" -> (0, 2, 3), "0a5" -> (0, 1, 5)
func parseVersionSuffix(part string) (num, releaseType, releaseNum int) {
	// Find where the letter starts
	letterIdx := -1
	for i, c := range part {
		if c == 'a' || c == 'b' || c == 'f' {
			letterIdx = i
			break
		}
	}

	if letterIdx == -1 {
		// No suffix, treat as final release
		_, _ = fmt.Sscanf(part, "%d", &num)
		return num, 3, 0
	}

	// Parse number before letter
	_, _ = fmt.Sscanf(part[:letterIdx], "%d", &num)

	// Parse release type
	switch part[letterIdx] {
	case 'a':
		releaseType = 1 // alpha
	case 'b':
		releaseType = 2 // beta
	case 'f':
		releaseType = 3 // final
	}

	// Parse release number after letter
	if letterIdx+1 < len(part) {
		_, _ = fmt.Sscanf(part[letterIdx+1:], "%d", &releaseNum)
	}

	return
}

// FilterReleasesByVersion filters releases that match a version prefix
func FilterReleasesByVersion(releases []UnityRelease, prefix string) []UnityRelease {
	if prefix == "" {
		return releases
	}

	prefix = strings.ToLower(prefix)
	var result []UnityRelease
	for _, r := range releases {
		if strings.Contains(strings.ToLower(r.Version), prefix) {
			result = append(result, r)
		}
	}
	return result
}

// GetMajorMinorFromVersion extracts major.minor from a version string
func GetMajorMinorFromVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}
