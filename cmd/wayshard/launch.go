package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// tuiCompanionName is the packaged OpenCode-derived TUI executable shipped
// alongside the wayshard CLI in official installs.
func tuiCompanionName() string {
	if runtime.GOOS == "windows" {
		return "wayshard-tui.exe"
	}
	return "wayshard-tui"
}

// resolveTUI locates the packaged TUI companion deterministically without a
// PATH search. It only accepts:
//   - an explicit WAYSHARD_TUI override, or
//   - a regular file named wayshard-tui in the same resolved directory as the
//     running wayshard executable.
//
// A symlinked companion is resolved but must still live in that directory.
func resolveTUI() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve wayshard executable: %w", err)
	}
	dir := filepath.Dir(exe)
	if resolved, rerr := filepath.EvalSymlinks(dir); rerr == nil {
		dir = resolved
	}

	candidate := filepath.Join(dir, tuiCompanionName())
	if override := os.Getenv("WAYSHARD_TUI"); override != "" {
		if !filepath.IsAbs(override) {
			override = filepath.Join(dir, override)
		}
		candidate = override
	}

	info, lerr := os.Lstat(candidate)
	if lerr != nil {
		return "", fmt.Errorf("Wayshard TUI not found at %s (install the wayshard-tui companion, or set WAYSHARD_TUI): %w", candidate, lerr)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, rerr := filepath.EvalSymlinks(candidate)
		if rerr != nil {
			return "", fmt.Errorf("resolve Wayshard TUI symlink: %w", rerr)
		}
		if filepath.Dir(resolved) != dir {
			return "", fmt.Errorf("refusing Wayshard TUI companion outside the install directory: %s", resolved)
		}
		candidate = resolved
		info, lerr = os.Lstat(candidate)
		if lerr != nil {
			return "", lerr
		}
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("Wayshard TUI companion is not a regular file: %s", candidate)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("Wayshard TUI companion is not executable: %s", candidate)
	}
	return candidate, nil
}

// launchTUI hands the interactive session to the packaged Wayshard TUI. The
// scriptable subcommands above are unaffected.
func launchTUI(server, token string) {
	companion, err := resolveTUI()
	if err != nil {
		fatal(err)
	}
	env := os.Environ()
	if server != "" {
		env = append(env, "WAYSHARD_URL="+server)
	}
	if token != "" {
		env = append(env, "WAYSHARD_TOKEN="+token)
	}
	if err := execReplace(companion, []string{companion}, env); err != nil {
		fatal(fmt.Errorf("launch Wayshard TUI: %w", err))
	}
}
