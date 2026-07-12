//go:build windows

package daemon

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestIsProcessAliveReturnsFalseForExitedProcessWithOpenHandle(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child process: %v", err)
	}
	defer func() {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("wait child process: %v", err)
		}
	}()

	time.Sleep(300 * time.Millisecond)

	if isProcessAlive(cmd.Process.Pid) {
		t.Fatalf("expected pid %d to be treated as exited", cmd.Process.Pid)
	}
}

func TestStopRemovesInfoForExitedProcessWithOpenHandle(t *testing.T) {
	cfg := testConfig(t)

	cmd := exec.Command("cmd", "/c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child process: %v", err)
	}
	defer func() {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("wait child process: %v", err)
		}
	}()

	time.Sleep(300 * time.Millisecond)

	info, err := cfg.resolveEndpoint()
	if err != nil {
		t.Fatalf("resolve endpoint: %v", err)
	}
	info.PID = cmd.Process.Pid

	if err := writeInfo(cfg, info); err != nil {
		t.Fatalf("write info: %v", err)
	}

	if err := Stop(context.Background(), cfg); err != nil {
		t.Fatalf("stop exited process: %v", err)
	}

	if _, err := ReadInfo(cfg); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadInfo() err = %v, want os.ErrNotExist", err)
	}
}
