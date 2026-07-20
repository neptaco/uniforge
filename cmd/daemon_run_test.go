package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/updater"
)

func TestNewDaemonMetaUsesCLIReleaseVersion(t *testing.T) {
	meta := newDaemonMeta("0.9.1")

	if meta.ProtocolVersion != bridge.ProtocolVersion {
		t.Fatalf("protocol version = %d, want %d", meta.ProtocolVersion, bridge.ProtocolVersion)
	}
	if meta.Version != "0.9.1" {
		t.Fatalf("version = %q, want %q", meta.Version, "0.9.1")
	}
}

func TestUnityPackageVersionProviderReturnsCachedValueBeforeRefresh(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	var releaseOnce sync.Once

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		if requests.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"tag_name":"v0.11.0"}`))
			return
		}

		close(refreshStarted)
		<-releaseRefresh
		_, _ = w.Write([]byte(`{"tag_name":"v0.12.0"}`))
	}))
	defer func() {
		releaseOnce.Do(func() { close(releaseRefresh) })
		server.Close()
	}()

	opts := updater.AutoCheckOptions{
		CachePath:     filepath.Join(t.TempDir(), updater.UnityPackageUpdateCacheFilename),
		CheckInterval: time.Hour,
		Now:           func() time.Time { return now },
		APIBase:       server.URL,
		HTTPClient:    server.Client(),
	}
	if err := updater.RefreshUnityPackageAutoCheck(context.Background(), opts); err != nil {
		t.Fatalf("seed Unity package update cache: %v", err)
	}

	now = now.Add(2 * time.Hour)
	provider := newUnityPackageVersionProvider(opts)
	result := make(chan string, 1)
	go func() { result <- provider() }()

	select {
	case got := <-result:
		if got != "0.11.0" {
			t.Fatalf("provider result = %q, want cached version %q", got, "0.11.0")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("provider blocked on the network refresh")
	}

	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	releaseOnce.Do(func() { close(releaseRefresh) })

	deadline := time.Now().Add(time.Second)
	for {
		decision, err := updater.PrepareUnityPackageAutoCheck(opts)
		if err != nil {
			t.Fatalf("read refreshed cache: %v", err)
		}
		if decision.LatestVersion == "0.12.0" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("refreshed latest version = %q, want %q", decision.LatestVersion, "0.12.0")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
