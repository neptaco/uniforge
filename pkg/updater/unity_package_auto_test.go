package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUnityPackageUpdateCacheFilename(t *testing.T) {
	if got, want := UnityPackageUpdateCacheFilename, "unity-package-update-check.json"; got != want {
		t.Fatalf("UnityPackageUpdateCacheFilename = %q, want %q", got, want)
	}
}

func TestUnityPackageAutoCheckCachesLatestVersionWithoutPrefix(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	cachePath := filepath.Join(t.TempDir(), UnityPackageUpdateCacheFilename)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/tags" || r.URL.Query().Get("per_page") != "100" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[{"name":"not-a-version"},{"name":"v0.11.0"},{"name":"v0.12.0"}]`))
	}))
	defer server.Close()

	opts := AutoCheckOptions{
		CachePath:  cachePath,
		Now:        func() time.Time { return now },
		APIBase:    server.URL,
		HTTPClient: server.Client(),
	}
	decision, err := PrepareUnityPackageAutoCheck(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.CheckDue || decision.LatestVersion != "" {
		t.Fatalf("first decision = %#v, want background check with no cached version", decision)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("PrepareUnityPackageAutoCheck made %d HTTP requests, want 0", got)
	}

	if err := RefreshUnityPackageAutoCheck(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("RefreshUnityPackageAutoCheck made %d HTTP requests, want 1", got)
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	var cached struct {
		LatestVersion string `json:"latest_version"`
	}
	if err := json.Unmarshal(data, &cached); err != nil {
		t.Fatal(err)
	}
	if got, want := cached.LatestVersion, "0.12.0"; got != want {
		t.Fatalf("cached latest_version = %q, want %q", got, want)
	}
	latestVersion, err := ReadUnityPackageLatestVersion(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := latestVersion, "0.12.0"; got != want {
		t.Fatalf("ReadUnityPackageLatestVersion = %q, want %q", got, want)
	}

	now = now.Add(time.Minute)
	decision, err = PrepareUnityPackageAutoCheck(opts)
	if err != nil {
		t.Fatal(err)
	}
	if decision.CheckDue || decision.LatestVersion != "0.12.0" {
		t.Fatalf("cached decision = %#v, want cached raw semver without refresh", decision)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("cached PrepareUnityPackageAutoCheck made an HTTP request; total = %d, want 1", got)
	}
}

func TestUnityPackageAutoCheckUsesUnityRepositoryByDefault(t *testing.T) {
	var requestedURL string
	client := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestedURL = request.URL.String()
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`[{"name":"v0.12.0"}]`)),
				Request:    request,
			}, nil
		}),
	}

	err := RefreshUnityPackageAutoCheck(context.Background(), AutoCheckOptions{
		CachePath:  filepath.Join(t.TempDir(), UnityPackageUpdateCacheFilename),
		HTTPClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := requestedURL, "https://api.github.com/repos/neptaco/uniforge-unity/tags?per_page=100"; got != want {
		t.Fatalf("package tags URL = %q, want %q", got, want)
	}
}

func TestUnityPackageAutoCheckRejectsTagsWithoutSemanticVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"main"},{"name":"preview"}]`))
	}))
	defer server.Close()

	err := RefreshUnityPackageAutoCheck(context.Background(), AutoCheckOptions{
		CachePath:  filepath.Join(t.TempDir(), UnityPackageUpdateCacheFilename),
		APIBase:    server.URL,
		HTTPClient: server.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "vX.Y.Z") {
		t.Fatalf("error = %v, want semantic-version guidance", err)
	}
}

func TestReadUnityPackageLatestVersionIgnoresInvalidVersion(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), UnityPackageUpdateCacheFilename)
	if err := os.WriteFile(cachePath, []byte(`{"schema_version":1,"latest_version":"latest"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	latestVersion, err := ReadUnityPackageLatestVersion(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if latestVersion != "" {
		t.Fatalf("ReadUnityPackageLatestVersion = %q, want empty for invalid cache value", latestVersion)
	}
}

func TestUnityPackageAutoCheckClaimIsExclusive(t *testing.T) {
	opts := AutoCheckOptions{
		CachePath: filepath.Join(t.TempDir(), UnityPackageUpdateCacheFilename),
		Now:       func() time.Time { return time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC) },
	}

	const workers = 12
	var wg sync.WaitGroup
	results := make(chan bool, workers)
	errors := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decision, err := PrepareUnityPackageAutoCheck(opts)
			if err != nil {
				errors <- err
				return
			}
			results <- decision.CheckDue
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Errorf("PrepareUnityPackageAutoCheck: %v", err)
	}

	claimed := 0
	for checkDue := range results {
		if checkDue {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("Unity package check claimed %d times, want 1", claimed)
	}
}

func TestUnityPackageVersionComparison(t *testing.T) {
	tests := []struct {
		candidate string
		current   string
		want      bool
	}{
		{candidate: "0.12.0", current: "0.11.0", want: true},
		{candidate: "v0.12.0", current: "0.11.0", want: true},
		{candidate: "0.12.0", current: "0.12.0", want: false},
		{candidate: "0.11.0", current: "0.12.0", want: false},
		{candidate: "latest", current: "0.11.0", want: false},
		{candidate: "0.12.0", current: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.candidate+"_vs_"+tt.current, func(t *testing.T) {
			if got := IsNewerVersion(tt.candidate, tt.current); got != tt.want {
				t.Errorf("IsNewerVersion(%q, %q) = %v, want %v", tt.candidate, tt.current, got, tt.want)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
