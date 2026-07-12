//go:build darwin || linux

package unity

import (
	"os/exec"
	"syscall"
)

func configureOpenCommand(cmd *exec.Cmd) {
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
