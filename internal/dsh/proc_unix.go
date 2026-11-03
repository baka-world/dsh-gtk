//go:build unix

package dsh

import (
	"os/exec"
	"syscall"
)

// configureProcess puts the server in a process group of its own, so that
// signals reach the helpers it starts too.
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcess(cmd *exec.Cmd) {
	signalGroup(cmd, syscall.SIGTERM)
}

func killProcess(cmd *exec.Cmd) {
	signalGroup(cmd, syscall.SIGKILL)
}

func signalGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, sig); err != nil {
		_ = cmd.Process.Signal(sig)
	}
}
