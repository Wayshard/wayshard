package sandbox

import (
	"os"
	"strings"
)

// ProcessClass selects which ambient environment keys a process may inherit.
type ProcessClass string

const (
	ClassHarness ProcessClass = "harness"
	ClassTool    ProcessClass = "tool"
)

// baseHarnessEnv lists ambient keys a harness sandbox may inherit. Everything
// else (including server secrets and unrelated host credentials) is dropped.
var baseHarnessEnv = map[string]struct{}{
	"PATH": {}, "HOME": {}, "LANG": {}, "LANGUAGE": {}, "LC_ALL": {}, "LC_CTYPE": {}, "TERM": {},
	"TZ": {}, "USER": {}, "LOGNAME": {}, "SHELL": {}, "SSL_CERT_FILE": {}, "SSL_CERT_DIR": {},
	"LOCALAPPDATA": {}, "APPDATA": {}, "SYSTEMROOT": {}, "WINDIR": {}, "COMSPEC": {},
	// Harness-owned provider configuration the harness itself needs.
	"OPENCODE_CONFIG": {}, "OPENCODE_CONFIG_DIR": {}, "XDG_CONFIG_HOME": {}, "XDG_DATA_HOME": {},
	"XDG_CACHE_HOME": {}, "CODEX_HOME": {},
}

// baseToolEnv lists ambient keys a tool sandbox may inherit.
var baseToolEnv = map[string]struct{}{
	"PATH": {}, "LANG": {}, "LANGUAGE": {}, "LC_ALL": {}, "LC_CTYPE": {}, "TERM": {},
	"TZ": {}, "USER": {}, "LOGNAME": {}, "SHELL": {}, "SSL_CERT_FILE": {}, "SSL_CERT_DIR": {},
	"LOCALAPPDATA": {}, "APPDATA": {}, "SYSTEMROOT": {}, "WINDIR": {}, "COMSPEC": {},
}

// BuildEnv constructs a confined environment for a process class from the
// current process environment plus explicit additive entries. It replaces
// blacklist filtering: only allowlisted ambient keys survive.
func BuildEnv(class ProcessClass, add map[string]string) []string {
	allow := baseHarnessEnv
	if class == ClassTool {
		allow = baseToolEnv
	}
	seen := map[string]struct{}{}
	var out []string
	for _, kv := range os.Environ() {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if _, ok := allow[k]; !ok {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, kv)
	}
	for k, v := range add {
		if _, dup := seen[k]; dup {
			for i := range out {
				if strings.HasPrefix(out[i], k+"=") {
					out[i] = k + "=" + v
				}
			}
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k+"="+v)
	}
	return out
}

// HarnessEnv returns an environment for a harness process, including a
// synthetic HOME/TMP and explicit additive entries.
func HarnessEnv(syntheticHome, syntheticTemp string, add map[string]string) []string {
	m := map[string]string{}
	for k, v := range add {
		m[k] = v
	}
	if syntheticHome != "" {
		m["HOME"] = syntheticHome
	}
	if syntheticTemp != "" {
		m["TMPDIR"] = syntheticTemp
		m["TEMP"] = syntheticTemp
		m["TMP"] = syntheticTemp
	}
	return BuildEnv(ClassHarness, m)
}

// ToolEnv returns an environment for a tool/validation process.
func ToolEnv(syntheticHome, syntheticTemp string, scoped map[string]string) []string {
	m := map[string]string{}
	for k, v := range scoped {
		m[k] = v
	}
	if syntheticHome != "" {
		m["HOME"] = syntheticHome
	}
	if syntheticTemp != "" {
		m["TMPDIR"] = syntheticTemp
		m["TEMP"] = syntheticTemp
		m["TMP"] = syntheticTemp
	}
	return BuildEnv(ClassTool, m)
}