//go:build darwin || linux

package unity

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func probeUnityLockfile(path string) (bool, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false, err
	}
	defer file.Close()

	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		if unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); unlockErr != nil {
			return false, fmt.Errorf("release probe lock: %w", unlockErr)
		}
		return false, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return true, nil
	}
	return false, fmt.Errorf("acquire probe lock: %w", err)
}
