//go:build windows

package unity

import "os/exec"

func configureOpenCommand(cmd *exec.Cmd) {
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
}
