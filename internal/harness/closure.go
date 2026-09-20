package harness

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// harnessClosure is the minimum additional launch closure for an installed
// harness whose executable is a script or a symlink to one (for example a
// Node-based ACP adapter): read-only roots for its package tree and interpreter,
// plus any interpreter directories that must be on PATH so a
// "#!/usr/bin/env <cmd>" launcher resolves.
//
// It deliberately never includes the user's home directory or a blanket
// node_modules tree outside the harness's own package.
type harnessClosure struct {
	Roots    []string
	PathDirs []string
}

func harnessClosureFor(exePath string) harnessClosure {
	var c harnessClosure
	if exePath == "" {
		return c
	}
	seen := map[string]struct{}{}
	addRoot := func(p string) {
		if p == "" || p == "." || p == string(os.PathSeparator) {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		c.Roots = append(c.Roots, p)
	}
	addPath := func(p string) {
		if p == "" {
			return
		}
		for _, existing := range c.PathDirs {
			if existing == p {
				return
			}
		}
		c.PathDirs = append(c.PathDirs, p)
	}

	addRoot(filepath.Dir(exePath))
	resolved := exePath
	if rp, err := filepath.EvalSymlinks(exePath); err == nil {
		resolved = rp
		addRoot(filepath.Dir(resolved))
	}

	scriptDir := filepath.Dir(resolved)
	if interp, needPATH := shebangInterpreter(resolved, scriptDir); interp != "" {
		if rp, err := filepath.EvalSymlinks(interp); err == nil {
			interp = rp
		}
		addRoot(filepath.Dir(interp))
		if needPATH {
			addPath(filepath.Dir(interp))
		}
	}

	if pkg := packageRoot(scriptDir); pkg != "" {
		addRoot(pkg)
	}
	return c
}

// shebangInterpreter returns the interpreter a script would run under, and
// whether that interpreter is resolved through PATH (an "env" shebang) and
// therefore must be present on the launched process's PATH.
func shebangInterpreter(path, scriptDir string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	line, err := bufio.NewReader(f).ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#!") {
		return "", false
	}
	fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "#!")))
	if len(fields) == 0 {
		return "", false
	}
	interp := fields[0]
	if filepath.Base(interp) == "env" {
		if len(fields) < 2 {
			return "", false
		}
		cmd := fields[1]
		if p, err := exec.LookPath(cmd); err == nil {
			return p, true
		}
		if p := findInAncestorBins(cmd, scriptDir); p != "" {
			return p, true
		}
		return "", false
	}
	return interp, false
}

// findInAncestorBins looks for <cmd> in a bin/ directory of the script's
// ancestors. This resolves a version-manager interpreter (for example an nvm
// node) without hard-coding any install path.
func findInAncestorBins(cmd, dir string) string {
	for i := 0; i < 16; i++ {
		cand := filepath.Join(dir, "bin", cmd)
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

// packageRoot returns the nearest ancestor directory (including dir) that looks
// like an npm/Node package root.
func packageRoot(dir string) string {
	for i := 0; i < 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

// harnessEnvPATH prepends any required interpreter directories to the ambient
// PATH for a harness launch.
func harnessEnvPATH(exePath, ambient string) string {
	c := harnessClosureFor(exePath)
	if len(c.PathDirs) == 0 {
		return ambient
	}
	sep := string(os.PathListSeparator)
	prefix := strings.Join(c.PathDirs, sep)
	if ambient == "" {
		return prefix
	}
	return prefix + sep + ambient
}

// harnessProcRoots returns the /proc root some runtimes (for example Bun/JSC)
// require at startup. For harness/probe policies this is safe because
// ProcIsolation gives the process a private procfs scoped to its own PID
// namespace; tools and validation never receive it.
func harnessProcRoots() []string {
	if st, err := os.Stat("/proc"); err == nil && st.IsDir() {
		return []string{"/proc"}
	}
	return nil
}
