//go:build windows

package platform

import "os"

func uidString() string {
	return os.Getenv("USERNAME")
}
