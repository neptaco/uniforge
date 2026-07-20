package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/daemon"
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

func TestDaemonRunStartsWhileParentHoldsLifecycleLock(t *testing.T) {
	if os.Getenv("UNIFORGE_DAEMON_RUN_HELPER") == "1" {
		cfg := daemon.Config{
			Name:       "uniforge-test",
			RuntimeDir: os.Getenv("UNIFORGE_DAEMON_RUNTIME_DIR"),
			StateDir:   os.Getenv("UNIFORGE_DAEMON_STATE_DIR"),
		}
		if err := runDaemon(cfg); err != nil {
			t.Fatalf("run daemon helper: %v", err)
		}
		return
	}

	dir, err := os.MkdirTemp("", "ufdr-")
	if err != nil {
		t.Fatalf("create daemon test directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfg := daemon.Config{
		Name:       "uniforge-test",
		RuntimeDir: filepath.Join(dir, "runtime"),
		StateDir:   filepath.Join(dir, "state"),
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := daemon.Start(ctx, cfg, daemon.StartOptions{
		Executable: executable,
		Args:       []string{"-test.run=^TestDaemonRunStartsWhileParentHoldsLifecycleLock$"},
		Env: []string{
			"UNIFORGE_DAEMON_RUN_HELPER=1",
			"UNIFORGE_DAEMON_RUNTIME_DIR=" + cfg.RuntimeDir,
			"UNIFORGE_DAEMON_STATE_DIR=" + cfg.StateDir,
		},
		WaitTimeout: 2 * time.Second,
	}); err != nil {
		logData, _ := os.ReadFile(filepath.Join(cfg.StateDir, "daemon.log"))
		t.Fatalf("start daemon runner: %v\ndaemon log:\n%s", err, logData)
	}
	t.Cleanup(func() {
		info, err := daemon.ReadInfo(cfg)
		if err != nil {
			return
		}
		process, err := os.FindProcess(info.PID)
		if err == nil {
			_ = process.Kill()
		}
	})

	info, err := daemon.ReadInfo(cfg)
	if err != nil {
		t.Fatalf("read daemon info: %v", err)
	}
	if info.Endpoint == "" {
		t.Fatal("daemon did not advertise an endpoint")
	}
}
