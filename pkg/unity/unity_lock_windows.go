//go:build windows

package unity

import (
	"errors"
	"fmt"
	"syscall"
)

const (
	errorSharingViolation syscall.Errno = 32
	errorLockViolation    syscall.Errno = 33
)

func probeUnityLockfile(path string) (bool, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, err
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
	if err == nil {
		if closeErr := syscall.CloseHandle(handle); closeErr != nil {
			return false, fmt.Errorf("close probe handle: %w", closeErr)
		}
		return false, nil
	}
	if errors.Is(err, errorSharingViolation) || errors.Is(err, errorLockViolation) {
		return true, nil
	}
	return false, fmt.Errorf("open lockfile exclusively: %w", err)
}
