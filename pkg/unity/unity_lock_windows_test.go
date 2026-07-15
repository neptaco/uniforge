//go:build windows

package unity

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestProbeUnityLockfileDistinguishesHeldAndStaleFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "UnityLockfile")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatalf("hold lockfile: %v", err)
	}

	held, err := probeUnityLockfile(path)
	if err != nil || !held {
		t.Fatalf("held = %v, err = %v; want held lock", held, err)
	}
	if err := syscall.CloseHandle(handle); err != nil {
		t.Fatalf("release lockfile: %v", err)
	}

	held, err = probeUnityLockfile(path)
	if err != nil || held {
		t.Fatalf("held = %v, err = %v; want stale lockfile", held, err)
	}
}
