package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	dir, err := os.MkdirTemp(os.TempDir(), "dtest-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return Config{
		Name:       "td",
		RuntimeDir: filepath.Join(dir, "rt"),
		StateDir:   filepath.Join(dir, "st"),
	}
}

func TestLockExclusive(t *testing.T) {
	cfg := testConfig(t)

	d1 := New(cfg)
	if err := d1.Lock(); err != nil {
		t.Fatalf("first lock: %v", err)
	}
	defer func() { _ = d1.Shutdown() }()

	d2 := New(cfg)
	if err := d2.Lock(); err == nil {
		_ = d2.Shutdown()
		t.Fatal("second lock should fail but succeeded")
	}
}

func TestLockReleasedAfterShutdown(t *testing.T) {
	cfg := testConfig(t)

	d1 := New(cfg)
	if err := d1.Lock(); err != nil {
		t.Fatalf("first lock: %v", err)
	}
	_ = d1.Shutdown()

	d2 := New(cfg)
	if err := d2.Lock(); err != nil {
		t.Fatalf("second lock after shutdown: %v", err)
	}
	_ = d2.Shutdown()
}

func TestListenRequiresLock(t *testing.T) {
	cfg := testConfig(t)
	d := New(cfg)
	if _, err := d.Listen(nil); err == nil {
		t.Fatal("Listen without Lock should fail")
	}
	if _, err := d.Listen(nil); err != ErrNotLocked {
		t.Fatalf("expected ErrNotLocked, got %v", err)
	}
}

func TestListenAndShutdown(t *testing.T) {
	cfg := testConfig(t)
	d := New(cfg)

	if err := d.Lock(); err != nil {
		t.Fatalf("lock: %v", err)
	}

	testMeta := json.RawMessage(`{"protocolVersion":1,"version":"test"}`)
	ln, err := d.Listen(testMeta)
	if err != nil {
		_ = d.Shutdown()
		t.Fatalf("listen: %v", err)
	}

	// Verify info file was written
	info, err := ReadInfo(cfg)
	if err != nil {
		_ = d.Shutdown()
		t.Fatalf("read info: %v", err)
	}
	if info.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", info.PID, os.Getpid())
	}

	// Verify metadata was stored correctly
	var meta struct {
		ProtocolVersion int    `json:"protocolVersion"`
		Version         string `json:"version"`
	}
	if err := json.Unmarshal(info.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta.ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion = %d, want 1", meta.ProtocolVersion)
	}
	if meta.Version != "test" {
		t.Errorf("Version = %q, want %q", meta.Version, "test")
	}

	// Verify listener works via platform-appropriate dial
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	conn := testDial(t, info.Endpoint)
	_ = conn.Close()

	// Shutdown
	_ = d.Shutdown()

	// Verify files cleaned up
	if _, err := ReadInfo(cfg); err == nil {
		t.Error("info file should be removed after shutdown")
	}
}

func TestIsRunningAndStop(t *testing.T) {
	cfg := testConfig(t)

	// Not running initially
	if IsRunning(cfg) {
		t.Fatal("should not be running")
	}

	// Stop on non-existent is safe
	if err := Stop(context.Background(), cfg); err != nil {
		t.Fatalf("stop non-existent: %v", err)
	}
}

func TestStartDaemonOutlivesCallerContext(t *testing.T) {
	if os.Getenv("UNIFORGE_DAEMON_START_HELPER") == "1" {
		runDaemonStartHelper(t)
		return
	}

	cfg := testConfig(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helperDir, err := os.MkdirTemp("", "daemon-start-helper-")
	if err != nil {
		t.Fatalf("create helper directory: %v", err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			err := os.RemoveAll(helperDir)
			if err == nil || time.Now().After(deadline) {
				if err != nil {
					t.Errorf("remove helper directory: %v", err)
				}
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	helperExecutable := filepath.Join(helperDir, filepath.Base(executable))
	shutdownFile := filepath.Join(helperDir, "shutdown")
	executableData, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read test executable: %v", err)
	}
	if err := os.WriteFile(helperExecutable, executableData, 0o755); err != nil {
		t.Fatalf("copy test executable: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := Start(ctx, cfg, StartOptions{
		Executable:  helperExecutable,
		Args:        []string{"-test.run=TestStartDaemonOutlivesCallerContext"},
		Env:         []string{"UNIFORGE_DAEMON_START_HELPER=1", "UNIFORGE_DAEMON_RUNTIME_DIR=" + cfg.RuntimeDir, "UNIFORGE_DAEMON_STATE_DIR=" + cfg.StateDir, "UNIFORGE_DAEMON_SHUTDOWN_FILE=" + shutdownFile},
		WaitTimeout: 5 * time.Second,
	}); err != nil {
		cancel()
		t.Fatalf("start helper daemon: %v", err)
	}
	info, err := ReadInfo(cfg)
	if err != nil {
		cancel()
		t.Fatalf("read helper daemon info: %v", err)
	}
	cancel()
	time.Sleep(200 * time.Millisecond)
	if !isProcessAlive(info.PID) {
		t.Fatal("daemon was killed when the caller context was canceled")
	}
	if err := os.WriteFile(shutdownFile, nil, 0o600); err != nil {
		_ = forceStop(info.PID)
		t.Fatalf("request helper shutdown: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := ReadInfo(cfg)
		if os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			_ = forceStop(info.PID)
			t.Fatal("helper daemon did not shut down")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func runDaemonStartHelper(t *testing.T) {
	t.Helper()
	cfg := Config{
		Name:       "td",
		RuntimeDir: os.Getenv("UNIFORGE_DAEMON_RUNTIME_DIR"),
		StateDir:   os.Getenv("UNIFORGE_DAEMON_STATE_DIR"),
	}
	d := New(cfg)
	if err := d.Lock(); err != nil {
		t.Fatalf("helper lock: %v", err)
	}
	if _, err := d.Listen(nil); err != nil {
		t.Fatalf("helper listen: %v", err)
	}
	shutdownFile := os.Getenv("UNIFORGE_DAEMON_SHUTDOWN_FILE")
	for {
		if _, err := os.Stat(shutdownFile); err == nil {
			if err := d.Shutdown(); err != nil {
				t.Fatalf("helper shutdown: %v", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStopRemovesStaleInfoWithoutSignalingPID(t *testing.T) {
	cfg := testConfig(t)
	if err := writeInfo(cfg, Info{PID: os.Getpid()}); err != nil {
		t.Fatalf("write stale info: %v", err)
	}

	if err := Stop(context.Background(), cfg); err != nil {
		t.Fatalf("stop stale daemon: %v", err)
	}
	if _, err := ReadInfo(cfg); !os.IsNotExist(err) {
		t.Fatalf("stale info should be removed, got %v", err)
	}
}

func TestStopRejectsInfoPIDThatDoesNotMatchLockOwner(t *testing.T) {
	cfg := testConfig(t)
	d := New(cfg)
	if err := d.Lock(); err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer func() { _ = d.Shutdown() }()

	if err := writeInfo(cfg, Info{PID: os.Getpid() + 1}); err != nil {
		t.Fatalf("write mismatched info: %v", err)
	}
	if err := Stop(context.Background(), cfg); err == nil {
		t.Fatal("Stop should reject a PID that does not own the daemon lock")
	}
}

func TestIsRunningWithLockedDaemon(t *testing.T) {
	cfg := testConfig(t)

	d := New(cfg)
	if err := d.Lock(); err != nil {
		t.Fatalf("lock: %v", err)
	}

	// Daemon holds the lock, so IsRunning should return true
	if !IsRunning(cfg) {
		t.Error("expected IsRunning=true while daemon holds the lock")
	}

	_ = d.Shutdown()

	// After shutdown, IsRunning should return false
	if IsRunning(cfg) {
		t.Error("expected IsRunning=false after shutdown")
	}
}

func TestConfigDefaultDirs(t *testing.T) {
	cfg := Config{Name: "myapp"}
	dir, err := cfg.runtimeDir()
	if err != nil {
		t.Fatalf("runtimeDir: %v", err)
	}
	if dir == "" {
		t.Fatal("runtimeDir should not be empty")
	}

	stateDir, err := cfg.stateDir()
	if err != nil {
		t.Fatalf("stateDir: %v", err)
	}
	if stateDir == "" {
		t.Fatal("stateDir should not be empty")
	}
}

func TestConfigOverrideDirs(t *testing.T) {
	cfg := Config{
		Name:       "myapp",
		RuntimeDir: filepath.Join(os.TempDir(), "custom-runtime"),
		StateDir:   filepath.Join(os.TempDir(), "custom-state"),
	}

	dir, _ := cfg.runtimeDir()
	if dir != cfg.RuntimeDir {
		t.Errorf("runtimeDir = %q, want %q", dir, cfg.RuntimeDir)
	}

	dir, _ = cfg.stateDir()
	if dir != cfg.StateDir {
		t.Errorf("stateDir = %q, want %q", dir, cfg.StateDir)
	}
}

func TestConfigValidate(t *testing.T) {
	if err := (Config{Name: "myapp"}).Validate(); err != nil {
		t.Errorf("valid config should not error: %v", err)
	}
	if err := (Config{}).Validate(); err == nil {
		t.Error("empty name should fail validation")
	}
}

func TestResolveEndpoint(t *testing.T) {
	cfg := testConfig(t)
	info, err := cfg.resolveEndpoint()
	if err != nil {
		t.Fatalf("resolveEndpoint: %v", err)
	}
	if info.Transport != expectedTransport() {
		t.Errorf("Transport = %q, want %q", info.Transport, expectedTransport())
	}
	if info.Endpoint == "" {
		t.Error("Endpoint should not be empty")
	}
}

func TestShutdownIdempotent(t *testing.T) {
	cfg := testConfig(t)
	d := New(cfg)
	if err := d.Lock(); err != nil {
		t.Fatalf("lock: %v", err)
	}

	// Multiple shutdowns should not panic
	_ = d.Shutdown()
	_ = d.Shutdown()
	_ = d.Shutdown()
}

func TestProcessAlive(t *testing.T) {
	// Current process should be alive
	if !isProcessAlive(os.Getpid()) {
		t.Error("current process should be alive")
	}

	// Non-existent PID should not be alive
	// Use a very high PID that is unlikely to exist
	if isProcessAlive(99999999) {
		t.Error("non-existent PID should not be alive")
	}
}
