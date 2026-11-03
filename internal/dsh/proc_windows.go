//go:build windows

package dsh

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}

// terminateProcess ends a Windows process: node has no signal to catch
// there, so the server is always killed rather than asked to exit.
func terminateProcess(cmd *exec.Cmd) { killProcess(cmd) }

func killProcess(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
