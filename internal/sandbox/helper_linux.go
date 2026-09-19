//go:build linux

package sandbox

import (
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func runHelper(args []string) bool {
	if len(args) < 5 || args[1] != HelperArg || args[3] != "--" {
		return false
	}
	policyFile := args[2]
	cmdPath := args[4]
	cmdArgs := args[4:]

	b, err := os.ReadFile(policyFile)
	_ = os.Remove(policyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-sandbox: read policy:", err)
		os.Exit(126)
	}
	var p Policy
	if err := json.Unmarshal(b, &p); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-sandbox: parse policy:", err)
		os.Exit(126)
	}
	if p.SyntheticHome != "" {
		_ = os.MkdirAll(p.SyntheticHome, 0o700)
	}
	if p.SyntheticTemp != "" {
		_ = os.MkdirAll(p.SyntheticTemp, 0o700)
	}
	if err := applyLandlock(p); err != nil {
		if p.Required {
			fmt.Fprintln(os.Stderr, "wayshard-sandbox: required confinement failed:", err)
			os.Exit(125)
		}
	}
	if err := unix.Exec(cmdPath, cmdArgs, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-sandbox: exec:", err)
		os.Exit(127)
	}
	return true
}
