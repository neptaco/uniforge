package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRunChecksAndInstallsRelease(t *testing.T) {
	archive := makeArchive(t, "uniforge", []byte("new binary"))
	sum := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
		case "/download/v1.2.3/uniforge_linux_amd64.tar.gz":
			_, _ = w.Write(archive)
		case "/download/v1.2.3/checksums.txt":
			_, _ = fmt.Fprintf(w, "%x  uniforge_linux_amd64.tar.gz\n", sum)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	executable := filepath.Join(dir, "uniforge")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), Options{
		CurrentVersion: "1.0.0",
		Executable:     executable,
		GOOS:           "linux",
		GOARCH:         "amd64",
		APIBase:        server.URL,
		DownloadBase:   server.URL + "/download",
		HTTPClient:     server.Client(),
		ValidateBinary: func(string, string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.TargetVersion != "v1.2.3" {
		t.Fatalf("unexpected result: %#v", result)
	}
	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Fatalf("installed content = %q", got)
	}
}

func TestRunCheckOnlyDoesNotReplaceExecutable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer server.Close()
	executable := filepath.Join(t.TempDir(), "uniforge")
	if err := os.WriteFile(executable, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), Options{
		CurrentVersion: "1.0.0",
		CheckOnly:      true,
		Executable:     executable,
		APIBase:        server.URL,
		HTTPClient:     server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || result.TargetVersion != "v1.2.3" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRunRejectsDevelopmentAndPackageManagerBuilds(t *testing.T) {
	_, err := Run(context.Background(), Options{CurrentVersion: "dev", Executable: "/tmp/uniforge"})
	if err == nil {
		t.Fatal("expected development build error")
	}
	_, err = Run(context.Background(), Options{CurrentVersion: "1.0.0", Executable: "/opt/homebrew/Cellar/uniforge/1.0/bin/uniforge"})
	if err == nil {
		t.Fatal("expected package manager error")
	}
}

func TestVerifyChecksumRejectsMismatch(t *testing.T) {
	if err := verifyChecksum("asset.tar.gz", []byte("content"), "deadbeef  asset.tar.gz\n"); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestAssetNames(t *testing.T) {
	tests := []struct {
		goos, goarch, archive, binary string
		wantErr                       bool
	}{
		{"darwin", "arm64", "uniforge_darwin_arm64.tar.gz", "uniforge", false},
		{"windows", "amd64", "uniforge_windows_amd64.tar.gz", "uniforge.exe", false},
		{"windows", "arm64", "", "", true},
		{"plan9", "amd64", "", "", true},
	}
	for _, tt := range tests {
		archive, binary, err := assetNames(tt.goos, tt.goarch)
		if (err != nil) != tt.wantErr || archive != tt.archive || binary != tt.binary {
			t.Fatalf("assetNames(%q, %q) = %q, %q, %v", tt.goos, tt.goarch, archive, binary, err)
		}
	}
}

func TestAutomaticUpdateCheckUsesCachedResultOnNextInvocation(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	cachePath := filepath.Join(t.TempDir(), "update-check.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer server.Close()

	opts := AutoCheckOptions{
		CurrentVersion: "1.0.0",
		CachePath:      cachePath,
		Now:            func() time.Time { return now },
		APIBase:        server.URL,
		HTTPClient:     server.Client(),
	}
	decision, err := PrepareAutoCheck(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.CheckDue || decision.Notice != nil {
		t.Fatalf("first decision = %#v, want background check only", decision)
	}
	if err := RefreshAutoCheck(context.Background(), opts); err != nil {
		t.Fatal(err)
	}

	now = now.Add(time.Minute)
	decision, err = PrepareAutoCheck(opts)
	if err != nil {
		t.Fatal(err)
	}
	if decision.CheckDue || decision.Notice == nil {
		t.Fatalf("cached decision = %#v, want notice without check", decision)
	}
	if decision.Notice.LatestVersion != "v1.2.3" || decision.Notice.CurrentVersion != "v1.0.0" {
		t.Fatalf("notice = %#v", decision.Notice)
	}

	if err := RecordAutoNotification(opts, decision.Notice.LatestVersion); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	decision, err = PrepareAutoCheck(opts)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Notice != nil {
		t.Fatalf("notice repeated before reminder interval: %#v", decision.Notice)
	}

	now = now.Add(8 * 24 * time.Hour)
	decision, err = PrepareAutoCheck(opts)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Notice == nil {
		t.Fatal("expected reminder after seven days")
	}
}

func TestAutomaticUpdateCheckClaimIsExclusive(t *testing.T) {
	opts := AutoCheckOptions{
		CurrentVersion: "v1.0.0",
		CachePath:      filepath.Join(t.TempDir(), "update-check.json"),
		Now:            func() time.Time { return time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC) },
	}
	const workers = 12
	var wg sync.WaitGroup
	results := make(chan bool, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decision, err := PrepareAutoCheck(opts)
			if err != nil {
				t.Errorf("PrepareAutoCheck: %v", err)
				return
			}
			results <- decision.CheckDue
		}()
	}
	wg.Wait()
	close(results)
	claimed := 0
	for checkDue := range results {
		if checkDue {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("check claimed %d times, want 1", claimed)
	}
}

func TestAutomaticUpdateVersionComparison(t *testing.T) {
	tests := []struct {
		candidate string
		current   string
		want      bool
	}{
		{"v1.2.3", "1.2.2", true},
		{"v2.0.0", "v1.99.99", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.2", "v1.2.3", false},
		{"latest", "v1.2.3", false},
		{"v1.2.3", "dev", false},
	}
	for _, tt := range tests {
		if got := isNewerVersion(tt.candidate, tt.current); got != tt.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tt.candidate, tt.current, got, tt.want)
		}
	}
}

func makeArchive(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
