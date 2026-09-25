//go:build windows

package process

import (
	"os"
	"os/exec"
	"syscall"
)

func configure(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

// KillTreePID terminates pid. Windows Job Objects would be needed to terminate
// the whole tree; this is the best-effort direct-child kill. Wayshard trusts the
// server OS user's own processes, so cleanup here is best-effort, not a
// containment guarantee.
func KillTreePID(pid int) {
	p, err := os.FindProcess(pid)
	if err != nil || p == nil {
		return
	}
	_ = p.Kill()
}

// TerminatePID terminates pid (Windows has no portable SIGTERM).
func TerminatePID(pid int) {
	KillTreePID(pid)
}
