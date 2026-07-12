//go:build !windows

package platform

import (
	"fmt"
	"os"
)

func uidString() string {
	return fmt.Sprintf("%d", os.Getuid())
}
