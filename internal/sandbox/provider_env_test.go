package sandbox

import (
	"strings"
	"testing"
)

// TestHarnessEnvProviderTransportWinsOverHostileParent proves the harness
// environment never inherits ambient proxy configuration and that the provider
// transport installed by Wayshard is authoritative. This closes the
// "host proxy env" bypass.
func TestHarnessEnvProviderTransportWinsOverHostileParent(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://hostile.example:1")
	t.Setenv("HTTPS_PROXY", "http://hostile.example:1")
	t.Setenv("ALL_PROXY", "socks5://hostile.example:1")
	t.Setenv("NO_PROXY", "*")

	ours := "http://127.0.0.1:34567"
	env := HarnessEnv("/home/user", "/tmp/synth", map[string]string{
		"HTTPS_PROXY": ours,
		"NO_PROXY":    "",
	})
	got := map[string]string{}
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			got[kv[:i]] = kv[i+1:]
		}
	}
	if got["HTTPS_PROXY"] != ours {
		t.Fatalf("HTTPS_PROXY = %q, want %q", got["HTTPS_PROXY"], ours)
	}
	if _, ok := got["HTTP_PROXY"]; ok {
		t.Fatalf("host HTTP_PROXY leaked into harness env: %q", got["HTTP_PROXY"])
	}
	if _, ok := got["ALL_PROXY"]; ok {
		t.Fatalf("host ALL_PROXY leaked into harness env: %q", got["ALL_PROXY"])
	}
	if v, ok := got["NO_PROXY"]; !ok || v != "" {
		t.Fatalf("NO_PROXY must be explicitly empty, got %q (present=%v)", v, ok)
	}
}
