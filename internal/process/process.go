// Package process provides best-effort process-tree management for
// server-launched commands (tool terminals and validation checks). Wayshard
// trusts the harnesses and tools it runs as the server OS user, so this is not a
// containment boundary: it only makes cancellation, timeout and shutdown clean
// up the process tree as best the platform allows.
package process

import "os/exec"

// Configure arranges for cmd to run in its own process group where the platform
// supports it, so the whole tree can be terminated together.
func Configure(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	configure(cmd)
}

// KillTree best-effort terminates cmd and its descendants. It never returns an
// error: cleanup must not block cancellation or shutdown.
func KillTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	KillTreePID(cmd.Process.Pid)
	_ = cmd.Process.Kill()
}
