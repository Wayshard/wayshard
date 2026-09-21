//go:build windows

package main

import (
	"os"
	"os/exec"
)

// execReplace runs the TUI as a child with inherited stdio and propagates its
// exit code. Windows has no exec-replace.
func execReplace(path string, argv []string, env []string) error {
	cmd := exec.Command(path, argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	os.Exit(0)
	return nil
}
