//go:build !windows

package unity

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func waitForProcessDeath(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if err := syscall.Kill(pid, 0); err != nil {
			return // ESRCH: process is gone
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d is still alive", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestReapProcessTreeKillsRealProcessTree(t *testing.T) {
	// sh spawns a grandchild sleep and prints its PID, then waits on it.
	cmd := exec.Command("sh", "-c", "sleep 30 & echo $!; wait")
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	buf := make([]byte, 32)
	n, err := output.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		t.Fatalf("failed to parse child pid: %v", err)
	}

	reaper := newProcessTreeReaper()
	descendants := reaper.snapshotDescendants(cmd.Process.Pid)
	found := false
	for _, process := range descendants {
		if process.PID == childPID {
			found = true
		}
	}
	if !found {
		t.Fatalf("descendants %v must contain child %d", descendants, childPID)
	}

	// Kill the root the same way Close --force does, orphaning the child.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	reaper.reap(descendants, 50*time.Millisecond, 2*time.Second)

	waitForProcessDeath(t, childPID, 2*time.Second)
}

func shortenTreeReapGraces(t *testing.T) {
	t.Helper()
	naturalGrace, termGrace := treeReapNaturalGrace, treeReapTermGrace
	treeReapNaturalGrace = 100 * time.Millisecond
	treeReapTermGrace = 500 * time.Millisecond
	t.Cleanup(func() {
		treeReapNaturalGrace, treeReapTermGrace = naturalGrace, termGrace
	})
}

// fakeUnityTree writes a fake Unity executable that spawns a grandchild sleep,
// records the grandchild PID, and blocks. Returns the script path and the file
// that will contain the grandchild PID.
func fakeUnityTree(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-unity.sh")
	childPIDFile := filepath.Join(dir, "child.pid")
	content := "#!/bin/sh\nsleep 30 &\necho $! > \"" + childPIDFile + "\"\nwait\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return script, childPIDFile
}

func readChildPID(t *testing.T, childPIDFile string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(childPIDFile)
		if err == nil && strings.TrimSpace(string(data)) != "" {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatalf("failed to parse child pid: %v", err)
			}
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatalf("child pid file %s was not written", childPIDFile)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Exercises the same ctx-done → Cancel path a batch timeout takes, but drives
// the cancellation actively after observing that the grandchild exists. A
// fixed timeout budget here would break under CI starvation (the script may
// not even have started when the deadline fires).
func TestUnityBatchCommandCancelReapsProcessTree(t *testing.T) {
	shortenTreeReapGraces(t)
	script, childPIDFile := fakeUnityTree(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := newUnityBatchCommand(ctx, script)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	childPID := readChildPID(t, childPIDFile)

	cancel()
	if err := cmd.Wait(); err == nil {
		t.Fatal("expected an error from the cancelled command")
	}
	waitForProcessDeath(t, childPID, 3*time.Second)
}
