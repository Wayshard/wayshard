//go:build unix

package acp

import (
	"os/exec"
	"syscall"
)

func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalTerm(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}

func killTree(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
