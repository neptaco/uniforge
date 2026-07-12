//go:build windows

package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
	processStillActive      = 259
)

var procLockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

// lockFile acquires a non-blocking exclusive lock using LockFileEx.
func lockFile(f *os.File) error {
	var ol syscall.Overlapped
	r1, _, err := procLockFileEx.Call(
		uintptr(f.Fd()),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 == 0 {
		return err
	}
	return nil
}

var procUnlockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("UnlockFileEx")

// unlockFile releases a previously acquired file lock.
func unlockFile(f *os.File) {
	var ol syscall.Overlapped
	_, _, _ = procUnlockFileEx.Call(
		uintptr(f.Fd()),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&ol)),
	)
}

// createListener creates a Windows named pipe listener.
func createListener(info Info) (net.Listener, error) {
	return winio.ListenPipe(info.Endpoint, nil)
}

// dialTransport connects to the daemon using the advertised transport.
func dialTransport(info Info, timeout time.Duration) (net.Conn, error) {
	switch info.Transport {
	case TransportNamedPipe:
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return winio.DialPipeContext(ctx, info.Endpoint)
	case TransportTCP:
		address := net.JoinHostPort(info.Host, fmt.Sprintf("%d", info.Port))
		return net.DialTimeout("tcp", address, timeout)
	default:
		return nil, fmt.Errorf("unsupported transport on this platform: %s", info.Transport)
	}
}

// processAttrs returns SysProcAttr for launching a daemon subprocess on Windows.
func processAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

const processQueryLimitedInformation = 0x1000

var procOpenProcess = syscall.NewLazyDLL("kernel32.dll").NewProc("OpenProcess")
var procGetExitCodeProcess = syscall.NewLazyDLL("kernel32.dll").NewProc("GetExitCodeProcess")

// isProcessAlive checks if a process with the given PID exists on Windows.
// OpenProcess alone is not enough because exited processes can still be opened
// while their process object remains alive. Query the exit code to distinguish
// a running process from an exited one.
func isProcessAlive(pid int) bool {
	handle, _, _ := procOpenProcess.Call(
		uintptr(processQueryLimitedInformation),
		0,
		uintptr(pid),
	)
	if handle == 0 {
		return false
	}
	defer func() {
		_ = syscall.CloseHandle(syscall.Handle(handle))
	}()

	var exitCode uint32
	r1, _, _ := procGetExitCodeProcess.Call(
		handle,
		uintptr(unsafe.Pointer(&exitCode)),
	)
	if r1 == 0 {
		return true
	}
	return exitCode == processStillActive
}

// gracefulStop attempts a graceful shutdown on Windows.
// Windows has no SIGTERM equivalent for detached processes, so this
// is best-effort: it tries GenerateConsoleCtrlEvent which only works
// for processes sharing the same console group. Returns an error if
// the process cannot be stopped gracefully (expected for daemon processes).
func gracefulStop(pid int) error {
	// GenerateConsoleCtrlEvent does not work for detached daemon processes.
	// Return an error to signal that the caller should proceed to forceStop.
	return fmt.Errorf("graceful stop not supported on Windows for detached processes")
}

// forceStop terminates the process immediately on Windows.
func forceStop(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func uidString() string { return os.Getenv("USERNAME") }
