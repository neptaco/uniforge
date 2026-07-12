package hub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetMajorMinorFromVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected string
	}{
		{"2022.3.60f1", "2022.3"},
		{"2021.3.5f1", "2021.3"},
		{"6000.0.32f1", "6000.0"},
		{"2023.1.0a1", "2023.1"},
		{"2022.3", "2022.3"},
		{"invalid", "invalid"}, // returns as-is if no dots
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := GetMajorMinorFromVersion(tt.version)
			if result != tt.expected {
				t.Errorf("GetMajorMinorFromVersion(%q) = %q, want %q", tt.version, result, tt.expected)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int // 1 if a > b, -1 if a < b, 0 if equal
	}{
		{"2022.3.60f1", "2022.3.59f1", 1},
		{"2022.3.59f1", "2022.3.60f1", -1},
		{"2022.3.60f1", "2022.3.60f1", 0},
		{"2023.1.0f1", "2022.3.60f1", 1},
		{"6000.0.1f1", "2022.3.60f1", 1},
		{"2022.3.60f1", "2021.3.60f1", 1},
		// Alpha/Beta/Final ordering
		{"6000.4.0f1", "6000.4.0b6", 1}, // final > beta
		{"6000.4.0b6", "6000.4.0a5", 1}, // beta > alpha
		{"6000.4.0b6", "6000.4.0b5", 1}, // b6 > b5
		{"6000.4.0b5", "6000.4.0b4", 1}, // b5 > b4
		{"6000.4.0a5", "6000.4.0a4", 1}, // a5 > a4
		{"6000.4.0a4", "6000.4.0a2", 1}, // a4 > a2
		{"6000.4.0b1", "6000.4.0a5", 1}, // beta > alpha even if number is lower
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			result := compareVersions(tt.a, tt.b)
			if (tt.expected > 0 && result <= 0) ||
				(tt.expected < 0 && result >= 0) ||
				(tt.expected == 0 && result != 0) {
				t.Errorf("compareVersions(%q, %q) = %d, want sign of %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestFilterReleasesByVersion(t *testing.T) {
	releases := []UnityRelease{
		{Version: "2022.3.60f1"},
		{Version: "2022.3.59f1"},
		{Version: "2022.3.5f1"},
		{Version: "2021.3.60f1"},
		{Version: "6000.0.32f1"},
	}

	tests := []struct {
		filter   string
		expected []string
	}{
		{"2022.3", []string{"2022.3.60f1", "2022.3.59f1", "2022.3.5f1"}},
		{"2022.3.6", []string{"2022.3.60f1"}},
		{"2022.3.5", []string{"2022.3.59f1", "2022.3.5f1"}},
		{"6000", []string{"6000.0.32f1"}},
		{"9999", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			result := FilterReleasesByVersion(releases, tt.filter)
			if len(result) != len(tt.expected) {
				t.Errorf("FilterReleasesByVersion filter=%q got %d results, want %d", tt.filter, len(result), len(tt.expected))
				return
			}
			for i, r := range result {
				if r.Version != tt.expected[i] {
					t.Errorf("FilterReleasesByVersion filter=%q result[%d] = %q, want %q", tt.filter, i, r.Version, tt.expected[i])
				}
			}
		})
	}
}

func TestMergeReleases(t *testing.T) {
	apiReleases := []UnityRelease{
		{
			Version:     "2022.3.60f1",
			Changeset:   "abc123",
			ReleaseDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Recommended: true,
		},
		{
			Version:   "2022.3.59f1",
			Changeset: "def456",
		},
	}

	localReleases := []UnityRelease{
		{
			Version: "2022.3.60f1",
			Modules: []ModuleInfo{{ID: "android", Name: "Android"}},
		},
		{
			Version: "2021.3.5f1",
			Modules: []ModuleInfo{{ID: "ios", Name: "iOS"}},
		},
	}

	result := mergeReleases(apiReleases, localReleases)

	// Should have 3 releases: 2 from API + 1 only in local
	if len(result) != 3 {
		t.Errorf("mergeReleases got %d releases, want 3", len(result))
	}

	// First release should have API metadata
	if result[0].Version != "2022.3.60f1" {
		t.Errorf("result[0].Version = %q, want %q", result[0].Version, "2022.3.60f1")
	}
	if result[0].Changeset != "abc123" {
		t.Errorf("result[0].Changeset = %q, want %q", result[0].Changeset, "abc123")
	}
	if !result[0].Recommended {
		t.Error("result[0].Recommended should be true")
	}
}

func TestDeduplicateReleases(t *testing.T) {
	releases := []UnityRelease{
		{Version: "2022.3.60f1", Changeset: ""},
		{Version: "2022.3.60f1", Changeset: "abc123"},
		{Version: "2022.3.59f1"},
		{Version: "2022.3.59f1", Modules: []ModuleInfo{{ID: "android"}}},
	}

	result := deduplicateReleases(releases)

	if len(result) != 2 {
		t.Errorf("deduplicateReleases got %d releases, want 2", len(result))
	}

	// Should keep the one with changeset
	for _, r := range result {
		if r.Version == "2022.3.60f1" && r.Changeset != "abc123" {
			t.Error("Should keep release with changeset")
		}
		if r.Version == "2022.3.59f1" && len(r.Modules) != 1 {
			t.Error("Should keep release with more modules")
		}
	}
}

func TestBuildStreamsFromReleases(t *testing.T) {
	releases := []UnityRelease{
		{Version: "2022.3.5f1", Stream: "TECH"},
		{Version: "2022.3.60f1", Stream: "LTS"},
		{Version: "2023.2.1f1", Stream: "TECH"},
		{Version: "6000.0.32f1", Stream: "LTS"},
		{Version: "6000.4.0a5", Stream: "BETA"},
		{Version: "6000.4.0b3", Stream: "BETA"},
	}
	totals := map[string]int{
		"2022.3":   120,
		"6000.0":   33,
		"6000.4":   0,
		"not.used": 1,
	}

	streams := BuildStreamsFromReleases(releases, totals)

	if len(streams) != 4 {
		t.Fatalf("BuildStreamsFromReleases got %d streams, want 4", len(streams))
	}

	expectedOrder := []string{"6000.4", "6000.0", "2023.2", "2022.3"}
	for i, expected := range expectedOrder {
		if streams[i].MajorMinor != expected {
			t.Fatalf("streams[%d].MajorMinor = %q, want %q", i, streams[i].MajorMinor, expected)
		}
	}

	byMajorMinor := make(map[string]VersionStream)
	for _, stream := range streams {
		byMajorMinor[stream.MajorMinor] = stream
	}

	tests := []struct {
		majorMinor    string
		displayName   string
		totalCount    int
		latestVersion string
		lts           bool
		isUnity6      bool
	}{
		{
			majorMinor:    "2022.3",
			displayName:   "2022.3 LTS",
			totalCount:    120,
			latestVersion: "2022.3.60f1",
			lts:           true,
			isUnity6:      false,
		},
		{
			majorMinor:    "2023.2",
			displayName:   "2023.2",
			totalCount:    1,
			latestVersion: "2023.2.1f1",
			lts:           false,
			isUnity6:      false,
		},
		{
			majorMinor:    "6000.0",
			displayName:   "Unity 6 (6000.0) LTS",
			totalCount:    33,
			latestVersion: "6000.0.32f1",
			lts:           true,
			isUnity6:      true,
		},
		{
			majorMinor:    "6000.4",
			displayName:   "Unity 6 (6000.4)",
			totalCount:    2,
			latestVersion: "6000.4.0b3",
			lts:           false,
			isUnity6:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.majorMinor, func(t *testing.T) {
			stream, ok := byMajorMinor[tt.majorMinor]
			if !ok {
				t.Fatalf("stream %q not found", tt.majorMinor)
			}
			if stream.DisplayName != tt.displayName {
				t.Errorf("DisplayName = %q, want %q", stream.DisplayName, tt.displayName)
			}
			if stream.TotalCount != tt.totalCount {
				t.Errorf("TotalCount = %d, want %d", stream.TotalCount, tt.totalCount)
			}
			if stream.LatestVersion != tt.latestVersion {
				t.Errorf("LatestVersion = %q, want %q", stream.LatestVersion, tt.latestVersion)
			}
			if stream.LTS != tt.lts {
				t.Errorf("LTS = %v, want %v", stream.LTS, tt.lts)
			}
			if stream.IsUnity6 != tt.isUnity6 {
				t.Errorf("IsUnity6 = %v, want %v", stream.IsUnity6, tt.isUnity6)
			}
		})
	}
}

func TestBuildStreamsFromReleases_Empty(t *testing.T) {
	streams := BuildStreamsFromReleases(nil, map[string]int{"2022.3": 1})
	if len(streams) != 0 {
		t.Errorf("BuildStreamsFromReleases empty got %d streams, want 0", len(streams))
	}
}

// withTestReleaseAPI points the shared Release API client at a test server
// (without pacing/retry delays) for the duration of the test
func withTestReleaseAPI(t *testing.T, serverURL string) {
	t.Helper()
	orig := defaultReleaseAPI
	defaultReleaseAPI = &releaseAPIClient{
		baseURL:    serverURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	t.Cleanup(func() { defaultReleaseAPI = orig })
}

// TestParseReleaseAPIResponse verifies that the real Release API response
// shape unmarshals into releaseAPIResponse and converts to UnityRelease.
func TestParseReleaseAPIResponse(t *testing.T) {
	client := &Client{}

	// Mirrors the actual response of GET /unity/editor/release/v1/releases
	responseJSON := `{
		"offset": 0,
		"limit": 1,
		"total": 65,
		"results": [
			{
				"version": "2022.3.62f3",
				"shortRevision": "96770f904ca7",
				"stream": "LTS",
				"releaseDate": "2025-10-28T10:40:42.860Z",
				"recommended": true,
				"unityHubDeepLink": "unityhub://2022.3.62f3/96770f904ca7",
				"skuFamily": "CLASSIC",
				"releaseNotes": {"url": "https://example.com/notes.md", "type": "MD"},
				"downloads": [
					{
						"url": "https://download.unity3d.com/download_unity/96770f904ca7/MacEditorInstallerArm64/Unity-2022.3.62f3.pkg",
						"type": "PKG",
						"platform": "MAC_OS",
						"architecture": "ARM64",
						"downloadSize": {"value": 4504652441, "unit": "BYTE"},
						"installedSize": {"value": 7826865000, "unit": "BYTE"},
						"modules": [
							{
								"id": "android",
								"slug": "2022.3.62f3-mac_os-arm64-android",
								"name": "Android Build Support",
								"description": "Allows building your Unity projects for the Android platform",
								"category": "PLATFORM",
								"url": "https://example.com/android.pkg",
								"type": "PKG",
								"downloadSize": {"value": 677455885, "unit": "BYTE"},
								"installedSize": {"value": 2196246000, "unit": "BYTE"},
								"required": false,
								"hidden": false,
								"preSelected": false,
								"subModules": []
							}
						]
					}
				]
			}
		]
	}`

	var resp releaseAPIResponse
	if err := json.Unmarshal([]byte(responseJSON), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Total != 65 {
		t.Errorf("Total = %d, want 65", resp.Total)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(resp.Results))
	}

	release := client.convertAPIRelease(resp.Results[0], "MAC_OS", "ARM64")

	if release.Version != "2022.3.62f3" {
		t.Errorf("Version = %q, want %q", release.Version, "2022.3.62f3")
	}
	if release.Changeset != "96770f904ca7" {
		t.Errorf("Changeset = %q, want %q", release.Changeset, "96770f904ca7")
	}
	if release.Stream != "LTS" {
		t.Errorf("Stream = %q, want %q", release.Stream, "LTS")
	}
	if !release.LTS {
		t.Error("LTS should be true")
	}
	if !release.Recommended {
		t.Error("Recommended should be true")
	}
	if release.ReleaseNotesURL != "https://example.com/notes.md" {
		t.Errorf("ReleaseNotesURL = %q, want %q", release.ReleaseNotesURL, "https://example.com/notes.md")
	}
	if release.DownloadSize != 4504652441 {
		t.Errorf("DownloadSize = %d, want %d", release.DownloadSize, int64(4504652441))
	}
	if len(release.Modules) != 1 {
		t.Fatalf("Expected 1 module, got %d", len(release.Modules))
	}
	if release.Modules[0].ID != "android" {
		t.Errorf("Module ID = %q, want %q", release.Modules[0].ID, "android")
	}
	if release.Modules[0].Category != "PLATFORM" {
		t.Errorf("Module Category = %q, want %q", release.Modules[0].Category, "PLATFORM")
	}
}

func TestModuleInfo_IsVisible(t *testing.T) {
	tests := []struct {
		name     string
		module   ModuleInfo
		expected bool
	}{
		{
			name:     "Platform not hidden",
			module:   ModuleInfo{Category: "PLATFORM", Hidden: false},
			expected: true,
		},
		{
			name:     "Platform hidden",
			module:   ModuleInfo{Category: "PLATFORM", Hidden: true},
			expected: false,
		},
		{
			name:     "DevTool not hidden",
			module:   ModuleInfo{Category: "DEV_TOOL", Hidden: false},
			expected: false,
		},
		{
			name:     "Documentation",
			module:   ModuleInfo{Category: "DOCUMENTATION", Hidden: false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.module.IsVisible()
			if result != tt.expected {
				t.Errorf("IsVisible() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConvertAPIRelease(t *testing.T) {
	client := &Client{}

	rel := releaseAPIRelease{
		Version:       "2022.3.60f1",
		ShortRevision: "abc123",
		Stream:        "LTS",
		ReleaseDate:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Recommended:   true,
		ReleaseNotes:  releaseAPIReleaseNotes{URL: "https://example.com"},
		Downloads: []releaseAPIDownload{
			{
				Platform:      "MAC_OS",
				Architecture:  "ARM64",
				DownloadSize:  digitalValue{Value: 2147483648},
				InstalledSize: digitalValue{Value: 4294967296},
				Modules: []releaseAPIModule{
					{
						ID:            "android",
						Name:          "Android Build Support",
						Category:      "PLATFORM",
						Hidden:        false,
						DownloadSize:  digitalValue{Value: 1073741824},
						InstalledSize: digitalValue{Value: 0},
					},
				},
			},
		},
	}

	release := client.convertAPIRelease(rel, "MAC_OS", "ARM64")

	if release.Version != "2022.3.60f1" {
		t.Errorf("Version = %q, want %q", release.Version, "2022.3.60f1")
	}
	if release.Changeset != "abc123" {
		t.Errorf("Changeset = %q, want %q", release.Changeset, "abc123")
	}
	if !release.LTS {
		t.Error("LTS should be true")
	}
	if !release.Recommended {
		t.Error("Recommended should be true")
	}
	if release.DownloadSize != 2147483648 {
		t.Errorf("DownloadSize = %d, want %d", release.DownloadSize, 2147483648)
	}
	if len(release.Modules) != 1 {
		t.Errorf("Expected 1 module, got %d", len(release.Modules))
	}
	if release.Modules[0].ID != "android" {
		t.Errorf("Module ID = %q, want %q", release.Modules[0].ID, "android")
	}
}

func TestConvertAPIRelease_DifferentPlatform(t *testing.T) {
	client := &Client{}

	rel := releaseAPIRelease{
		Version: "2022.3.60f1",
		Downloads: []releaseAPIDownload{
			{
				Platform:      "WINDOWS",
				Architecture:  "X86_64",
				DownloadSize:  digitalValue{Value: 1000000},
				InstalledSize: digitalValue{Value: 0},
			},
		},
	}

	// Request for MAC_OS but only WINDOWS available
	release := client.convertAPIRelease(rel, "MAC_OS", "ARM64")

	if release.DownloadSize != 0 {
		t.Errorf("DownloadSize should be 0 when platform doesn't match, got %d", release.DownloadSize)
	}
}

// TestFetchReleases_PaginationAndRetry verifies that FetchReleases pages
// through results (limit <= 25) and retries once on HTTP 429.
func TestFetchReleases_PaginationAndRetry(t *testing.T) {
	const total = 30

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		q := r.URL.Query()
		if q.Get("order") != "RELEASE_DATE_DESC" {
			t.Errorf("order = %q, want RELEASE_DATE_DESC", q.Get("order"))
		}
		if q.Get("version") != "2022.3" {
			t.Errorf("version = %q, want 2022.3", q.Get("version"))
		}

		limit, err := strconv.Atoi(q.Get("limit"))
		if err != nil || limit < 1 || limit > 25 {
			t.Errorf("limit = %q, must be an integer within [1, 25]", q.Get("limit"))
			limit = 25
		}
		offset, _ := strconv.Atoi(q.Get("offset"))

		var results []string
		for i := offset; i < offset+limit && i < total; i++ {
			results = append(results, fmt.Sprintf(`{
				"version": "2022.3.%df1",
				"shortRevision": "rev%d",
				"stream": "LTS",
				"releaseDate": "2024-01-15T00:00:00.000Z",
				"recommended": false,
				"releaseNotes": {"url": "https://example.com/notes.md", "type": "MD"},
				"downloads": []
			}`, total-i, i))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"offset": %d, "limit": %d, "total": %d, "results": [%s]}`,
			offset, limit, total, strings.Join(results, ","))
	}))
	defer server.Close()

	withTestReleaseAPI(t, server.URL)

	client := &Client{}
	releases, totals, err := client.fetchReleasesAndTotals([]string{"2022.3"})
	if err != nil {
		t.Fatalf("fetchReleasesAndTotals failed: %v", err)
	}

	if len(releases) != total {
		t.Errorf("Expected %d releases, got %d", total, len(releases))
	}
	if totals["2022.3"] != total {
		t.Errorf("totals[2022.3] = %d, want %d", totals["2022.3"], total)
	}

	seen := make(map[string]bool)
	for _, r := range releases {
		if seen[r.Version] {
			t.Errorf("Duplicate version %q across pages", r.Version)
		}
		seen[r.Version] = true
	}
}

// TestFetchReleases_PartialFailure verifies that failures for individual
// versions are tolerated as long as at least one version succeeds.
func TestFetchReleases_PartialFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("version") == "2021.3" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"total": 1, "results": [
			{
				"version": "2022.3.62f3",
				"shortRevision": "96770f904ca7",
				"stream": "LTS",
				"releaseDate": "2025-10-28T10:40:42.860Z",
				"recommended": false,
				"releaseNotes": {"url": "https://example.com/notes.md", "type": "MD"},
				"downloads": []
			}
		]}`)
	}))
	defer server.Close()

	withTestReleaseAPI(t, server.URL)

	client := &Client{}
	releases, err := client.FetchReleases([]string{"2022.3", "2021.3"})
	if err != nil {
		t.Fatalf("FetchReleases should tolerate a partial failure, got: %v", err)
	}
	if len(releases) != 1 || releases[0].Version != "2022.3.62f3" {
		t.Errorf("Expected the successful version's release, got %+v", releases)
	}
}

// TestReleaseAPIClient_WaitTurnPacing verifies that concurrent callers are
// spaced out by at least minInterval per request.
func TestReleaseAPIClient_WaitTurnPacing(t *testing.T) {
	api := &releaseAPIClient{minInterval: 20 * time.Millisecond}

	const calls = 3
	start := time.Now()
	var wg sync.WaitGroup
	for range calls {
		wg.Go(api.waitTurn)
	}
	wg.Wait()

	// The first call passes immediately; each subsequent call waits at
	// least minInterval after the previous one
	if elapsed, want := time.Since(start), (calls-1)*20*time.Millisecond; elapsed < time.Duration(want) {
		t.Errorf("%d waitTurn calls finished in %v, want at least %v", calls, elapsed, want)
	}
}

// TestFetchReleases_HTTPError verifies that a non-2xx response surfaces as an
// error instead of silently returning zero releases.
func TestFetchReleases_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"status":404,"title":"Not Found","code":54}`)
	}))
	defer server.Close()

	withTestReleaseAPI(t, server.URL)

	client := &Client{}
	_, err := client.FetchReleases([]string{"2022.3"})
	if err == nil {
		t.Fatal("Expected error for HTTP 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Error should mention status code, got: %v", err)
	}
}

// TestFetchReleases_Integration tests the actual Unity Release API.
// Skip in CI, run manually with: go test -run Integration -v ./pkg/hub/
func TestFetchReleases_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := &Client{}
	releases, err := client.FetchReleases([]string{"2022.3"})
	if err != nil {
		t.Fatalf("FetchReleases failed: %v", err)
	}

	t.Logf("Got %d releases", len(releases))

	// 2022.3 is a closed LTS stream with 60+ releases
	if len(releases) < 60 {
		t.Errorf("Expected at least 60 releases for 2022.3, got %d", len(releases))
	}

	for i, r := range releases {
		if i < 3 {
			t.Logf("Release: %s (changeset=%s, stream=%s, date=%s, rec=%v, size=%d, modules=%d)",
				r.Version, r.Changeset, r.Stream, r.ReleaseDate.Format("2006-01-02"), r.Recommended, r.DownloadSize, len(r.Modules))
		}
		if r.Version == "" {
			t.Error("Version should not be empty")
		}
		if r.Changeset == "" {
			t.Errorf("Changeset should not be empty for %s", r.Version)
		}
		if r.Stream == "" {
			t.Errorf("Stream should not be empty for %s", r.Version)
		}
	}
}

// TestFetchStreamMetadata_Integration verifies stream metadata against the
// actual Unity Release API.
func TestFetchStreamMetadata_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := &Client{}
	stream, err := client.fetchStreamMetadata("2022.3")
	if err != nil {
		t.Fatalf("fetchStreamMetadata failed: %v", err)
	}

	t.Logf("Stream: %+v", stream)

	if stream.TotalCount < 60 {
		t.Errorf("TotalCount = %d, want >= 60", stream.TotalCount)
	}
	if !strings.HasPrefix(stream.LatestVersion, "2022.3.") {
		t.Errorf("LatestVersion = %q, want 2022.3.x", stream.LatestVersion)
	}
	if !stream.LTS {
		t.Error("2022.3 should be LTS")
	}
}

// TestDiscoverMajorVersions_Integration verifies that major version discovery
// picks up current majors from the actual Unity Release API.
func TestDiscoverMajorVersions_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := &Client{NoCache: true}
	versions, err := client.fetchMajorVersionsFromAPI()
	if err != nil {
		t.Fatalf("fetchMajorVersionsFromAPI failed: %v", err)
	}

	t.Logf("Discovered major versions: %v", versions)

	if len(versions) == 0 {
		t.Fatal("Expected at least one major version")
	}

	// The latest LTS stream (Unity 6.x = 6000.x) must be discoverable
	hasUnity6 := false
	for _, v := range versions {
		if strings.HasPrefix(v, "6000.") {
			hasUnity6 = true
			break
		}
	}
	if !hasUnity6 {
		t.Error("Expected at least one 6000.x major version")
	}
}

func TestReleaseCacheRoundTrip(t *testing.T) {
	original := UnityRelease{
		Version:         "2022.3.60f1",
		Changeset:       "abc123",
		LTS:             true,
		Stream:          "LTS",
		ReleaseDate:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Recommended:     true,
		ReleaseNotesURL: "https://example.com/notes",
		DownloadSize:    2147483648,
		InstalledSize:   4294967296,
		Modules: []ModuleInfo{
			{
				ID:            "android",
				Name:          "Android Build Support",
				Category:      "PLATFORM",
				Hidden:        false,
				DownloadSize:  1073741824,
				InstalledSize: 2147483648,
			},
		},
	}

	// Convert to cache entry
	entry := releaseCacheEntry{
		Version:         original.Version,
		Changeset:       original.Changeset,
		LTS:             original.LTS,
		Stream:          original.Stream,
		ReleaseDate:     original.ReleaseDate,
		Recommended:     original.Recommended,
		ReleaseNotesURL: original.ReleaseNotesURL,
		DownloadSize:    original.DownloadSize,
		InstalledSize:   original.InstalledSize,
	}
	for _, mod := range original.Modules {
		entry.Modules = append(entry.Modules, moduleCacheEntry{
			ID:            mod.ID,
			Name:          mod.Name,
			Category:      mod.Category,
			Hidden:        mod.Hidden,
			DownloadSize:  mod.DownloadSize,
			InstalledSize: mod.InstalledSize,
		})
	}

	// Serialize and deserialize
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed releaseCacheEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify fields
	if parsed.Version != original.Version {
		t.Errorf("Version = %q, want %q", parsed.Version, original.Version)
	}
	if parsed.Changeset != original.Changeset {
		t.Errorf("Changeset = %q, want %q", parsed.Changeset, original.Changeset)
	}
	if !parsed.ReleaseDate.Equal(original.ReleaseDate) {
		t.Errorf("ReleaseDate = %v, want %v", parsed.ReleaseDate, original.ReleaseDate)
	}
	if parsed.Recommended != original.Recommended {
		t.Errorf("Recommended = %v, want %v", parsed.Recommended, original.Recommended)
	}
	if parsed.DownloadSize != original.DownloadSize {
		t.Errorf("DownloadSize = %d, want %d", parsed.DownloadSize, original.DownloadSize)
	}
	if len(parsed.Modules) != len(original.Modules) {
		t.Errorf("Modules length = %d, want %d", len(parsed.Modules), len(original.Modules))
	}
}
