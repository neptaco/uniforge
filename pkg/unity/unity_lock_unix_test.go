//go:build darwin || linux

package unity

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestProbeUnityLockfileDistinguishesHeldAndStaleFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "UnityLockfile")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("hold lockfile: %v", err)
	}
	held, err := probeUnityLockfile(path)
	if err != nil || !held {
		t.Fatalf("held = %v, err = %v; want held lock", held, err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatalf("release lockfile: %v", err)
	}
	held, err = probeUnityLockfile(path)
	if err != nil || held {
		t.Fatalf("held = %v, err = %v; want stale lockfile", held, err)
	}
}
