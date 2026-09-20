package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// maxLoginPATHBytes bounds the accepted login-shell PATH string.
const maxLoginPATHBytes = 32 << 10

// loginShellStartupFiles returns the specific startup files that a login
// invocation of the given shell may source. It deliberately does not grant the
// home directory or a whole /etc: only the well-known per-shell startup files
// that are actually required to compute PATH are exposed, and only when they
// exist. A login profile therefore cannot read unrelated private files such as
// ~/.aws-credentials, ~/.ssh/* or ~/.git-credentials.
func loginShellStartupFiles(shellPath, homeDir, etcDir string) []string {
	base := strings.ToLower(filepath.Base(shellPath))
	var cands []string
	add := func(p string) {
		if p != "" {
			cands = append(cands, p)
		}
	}

	if etcDir != "" {
		// POSIX login shells read /etc/profile. It commonly sources the files
		// in /etc/profile.d; those are enumerated individually rather than
		// granting the directory so unrelated files in the same tree stay
		// denied.
		add(filepath.Join(etcDir, "profile"))
		if entries, err := os.ReadDir(filepath.Join(etcDir, "profile.d")); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				add(filepath.Join(etcDir, "profile.d", e.Name()))
			}
		}
	}

	switch base {
	case "bash":
		add(filepath.Join(etcDir, "bash.bashrc"))
		add(filepath.Join(homeDir, ".bash_profile"))
		add(filepath.Join(homeDir, ".bash_login"))
		add(filepath.Join(homeDir, ".profile"))
		add(filepath.Join(homeDir, ".bashrc"))
	case "zsh":
		add(filepath.Join(etcDir, "zprofile"))
		add(filepath.Join(etcDir, "zshrc"))
		add(filepath.Join(etcDir, "zshenv"))
		add(filepath.Join(etcDir, "zlogin"))
		add(filepath.Join(homeDir, ".zprofile"))
		add(filepath.Join(homeDir, ".zshrc"))
		add(filepath.Join(homeDir, ".zshenv"))
		add(filepath.Join(homeDir, ".zlogin"))
	case "fish":
		add(filepath.Join(etcDir, "fish", "config.fish"))
		add(filepath.Join(homeDir, ".config", "fish", "config.fish"))
	case "ksh", "mksh", "ksh93", "pdksh":
		add(filepath.Join(homeDir, ".profile"))
		add(filepath.Join(homeDir, ".kshrc"))
	default:
		// sh, dash, ash, busybox sh and unknown POSIX-like shells.
		add(filepath.Join(homeDir, ".profile"))
	}

	var out []string
	for _, c := range cands {
		if c == "" {
			continue
		}
		st, err := os.Stat(c)
		if err != nil || st.IsDir() {
			continue
		}
		out = append(out, c)
	}
	return out
}

// LoginShellPolicy confines a login-shell PATH probe. It is a ProbePolicy
// variant with one bounded exception: the shell executable directory and the
// specific per-shell startup files under etcDir/homeDir are readable so the
// shell can source them. It is still NetworkNone, never exposes the Wayshard
// data dir or the project, and does not grant the home directory itself.
func LoginShellPolicy(shellPath, homeDir, etcDir, syntheticHome, syntheticTemp string) Policy {
	roots := systemReadOnlyRoots()
	// Resolve symlinked shells so the real target directory is readable.
	if rp, err := filepath.EvalSymlinks(shellPath); err == nil {
		shellPath = rp
	}
	if d := filepath.Dir(shellPath); d != "" && d != "." {
		roots = append(roots, d)
	}
	roots = append(roots, loginShellStartupFiles(shellPath, homeDir, etcDir)...)
	return Policy{
		ReadOnlyRoots:  roots,
		ReadWriteRoots: append([]string{syntheticHome, syntheticTemp}, systemDeviceRoots()...),
		SyntheticHome:  syntheticHome,
		SyntheticTemp:  syntheticTemp,
		Network:        NetNone,
		MemoryBytes:    512 << 20,
		WallTimeSec:    10,
		MaxProcesses:   64,
		MaxOutputBytes: 64 << 10,
		Required:       true,
	}
}

// SanitizeLoginPATH validates untrusted login-shell output and returns a
// normalized PATH. Only non-empty absolute path entries survive; relative,
// current-directory, control-character and duplicate entries are dropped.
// A result with no usable entries is an error, so discovery cannot fall back
// to executing from the current directory.
func SanitizeLoginPATH(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty login-shell PATH")
	}
	if len(raw) > maxLoginPATHBytes {
		return "", fmt.Errorf("login-shell PATH too large")
	}
	if strings.ContainsAny(raw, "\x00\n\r") {
		return "", fmt.Errorf("login-shell PATH contains line/control separators")
	}
	sep := string(os.PathListSeparator)
	seen := map[string]struct{}{}
	var out []string
	for _, part := range strings.Split(raw, sep) {
		if part == "" {
			continue
		}
		if strings.ContainsFunc(part, unicode.IsControl) {
			return "", fmt.Errorf("login-shell PATH entry contains control characters")
		}
		if !filepath.IsAbs(part) {
			// Drops empty, ".", "..", "~" and any cwd/project-relative entry.
			continue
		}
		clean := filepath.Clean(part)
		if clean == "" || clean == "." {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	if len(out) == 0 {
		return "", fmt.Errorf("login-shell PATH had no absolute entries")
	}
	return strings.Join(out, sep), nil
}
