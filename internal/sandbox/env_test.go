package sandbox

import (
	"strings"
	"testing"
)

// TestProbeEnvDropsHarnessConfig proves a discovery probe environment cannot
// read the user's real harness configuration, while a real harness run may.
func TestProbeEnvDropsHarnessConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/real-xdg-config")
	t.Setenv("XDG_DATA_HOME", "/tmp/real-xdg-data")
	t.Setenv("XDG_CACHE_HOME", "/tmp/real-xdg-cache")
	t.Setenv("OPENCODE_CONFIG", "/tmp/real-opencode.json")
	t.Setenv("OPENCODE_CONFIG_DIR", "/tmp/real-opencode-dir")
	t.Setenv("CODEX_HOME", "/tmp/real-codex")
	t.Setenv("LANG", "C.UTF-8")

	harnessEnv := strings.Join(HarnessEnv("/h", "/t", nil), "\n")
	if !strings.Contains(harnessEnv, "XDG_CONFIG_HOME=/tmp/real-xdg-config") {
		t.Fatalf("harness run should inherit harness config keys:\n%s", harnessEnv)
	}

	probeEnv := strings.Join(ProbeEnv("/h", "/t", nil), "\n")
	for _, k := range []string{"XDG_CONFIG_HOME=", "XDG_DATA_HOME=", "XDG_CACHE_HOME=", "OPENCODE_CONFIG=", "OPENCODE_CONFIG_DIR=", "CODEX_HOME="} {
		if strings.Contains(probeEnv, k) {
			t.Fatalf("probe env must not contain %q:\n%s", k, probeEnv)
		}
	}
	if !strings.Contains(probeEnv, "LANG=C.UTF-8") {
		t.Fatalf("probe env lost non-config allowlisted keys:\n%s", probeEnv)
	}
	if !strings.Contains(probeEnv, "HOME=/h") {
		t.Fatalf("probe env lost synthetic HOME:\n%s", probeEnv)
	}
}
