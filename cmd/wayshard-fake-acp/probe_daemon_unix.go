//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// spawnDetached starts a session-detached descendant that runs until killed.
// It simulates a probe that daemonizes a grandchild beyond its process group.
func spawnDetached(marker string) {
	c := exec.Command("/bin/sh", "-c", "while true; do sleep 0.5; done # "+marker)
	c.Env = os.Environ()
	c.Stdin = nil
	c.Stdout = nil
	c.Stderr = nil
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	_ = c.Start()
}
