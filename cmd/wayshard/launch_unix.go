//go:build !windows

package main

import "syscall"

// execReplace replaces the current process with the TUI so it owns the
// terminal directly (no extra process, no shell).
func execReplace(path string, argv []string, env []string) error {
	return syscall.Exec(path, argv, env)
}
