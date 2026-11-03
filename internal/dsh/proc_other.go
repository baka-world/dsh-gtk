//go:build !unix && !windows

package dsh

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}

func terminateProcess(cmd *exec.Cmd) { killProcess(cmd) }

func killProcess(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
