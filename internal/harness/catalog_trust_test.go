package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
)

func catBuild(t *testing.T, user string) *Catalog {
	t.Helper()
	cat, err := buildCatalog(shippedCatalogTOML, user)
	if err != nil {
		t.Fatalf("buildCatalog: %v", err)
	}
	return cat
}

func defOf(t *testing.T, cat *Catalog, id string) Definition {
	t.Helper()
	d, ok := cat.ByID(id)
	if !ok {
		t.Fatalf("definition %q missing (diagnostics: %v)", id, cat.Diagnostics)
	}
	return d
}

func TestTrustDefaultShipped(t *testing.T) {
	cat := catBuild(t, "")
	for _, id := range []string{"opencode", "codex"} {
		if got := VerifiedTransport(defOf(t, cat, id)); got != domain.TransportHTTPProxy {
			t.Fatalf("%s default transport = %s, want http_proxy", id, got)
		}
	}
}

func TestTrustCosmeticOverrideRetains(t *testing.T) {
	cat := catBuild(t, "schema_version=1\n[[harness]]\nid=\"opencode\"\ndisplay_name=\"Custom\"\nhomepage=\"https://example.invalid\"\n")
	if got := VerifiedTransport(defOf(t, cat, "opencode")); got != domain.TransportHTTPProxy {
		t.Fatalf("cosmetic override transport = %s, want http_proxy", got)
	}
}

func TestTrustMaterialOverridesLose(t *testing.T) {
	cases := map[string]string{
		"executable":                `executables=["other-binary"]`,
		"acp_args":                  `acp_args=["other","args"]`,
		"mode_bridge":               "acp=\"bridge\"\nbridges=[\"other-bridge\"]",
		"bridge_added":              `bridges=["other-bridge"]`,
		"bridge_args":               `bridge_args=["--x"]`,
		"loopback":                  `acp_requires_loopback=false`,
		"interpose":                 `interpose_commands=false`,
		"model_selection":           `model_selection="set_model"`,
		"config_roots":              `config_roots=[".config/opencode"]`,
		"well_known":                `well_known=[".config/opencode"]`,
		"version_args":              `version_args=[]`,
		"platforms":                 `platforms=["linux"]`,
		"requires_provider_network": `requires_provider_network=false`,
		"transport":                 `transport="none"`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			cat := catBuild(t, "schema_version=1\n[[harness]]\nid=\"opencode\"\n"+body+"\n")
			d := defOf(t, cat, "opencode")
			if got := VerifiedTransport(d); got != domain.TransportUnknown {
				t.Fatalf("material override %q retained transport %s", body, got)
			}
		})
	}
}

func TestTrustCodexMaterialOverridesLose(t *testing.T) {
	cases := map[string]string{
		"executable":      `executables=["other"]`,
		"bridge":          `bridges=["other-bridge"]`,
		"bridge_args":     `bridge_args=["--x"]`,
		"mode_native":     `acp="native"`,
		"model_selection": `model_selection="config_option"`,
		"interpose":       `interpose_commands=false`,
		"config_roots":    `config_roots=[".config/codex"]`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			cat := catBuild(t, "schema_version=1\n[[harness]]\nid=\"codex\"\n"+body+"\n")
			if got := VerifiedTransport(defOf(t, cat, "codex")); got != domain.TransportUnknown {
				t.Fatalf("codex material override %q retained transport %s", body, got)
			}
		})
	}
}

func TestTrustCustomReusedTrustedIDLoses(t *testing.T) {
	// A user entry that keeps the trusted id but points at a wholly different
	// implementation must not inherit trust.
	cat := catBuild(t, `schema_version=1
[[harness]]
id="opencode"
executables=["evil"]
acp="native"
acp_args=["--evil"]
config_roots=[".config/evil"]
`)
	if got := VerifiedTransport(defOf(t, cat, "opencode")); got != domain.TransportUnknown {
		t.Fatalf("redefined opencode retained transport %s", got)
	}
}

func TestFingerprintDeterministicAndOrderIndependent(t *testing.T) {
	a := catBuild(t, "schema_version=1\n[[harness]]\nid=\"opencode\"\ndisplay_name=\"A\"\n")
	b := catBuild(t, "schema_version=1\n[[harness]]\nid=\"opencode\"\nhomepage=\"https://x\"\ndisplay_name=\"B\"\n")
	if defOf(t, a, "opencode").ExecutionFingerprint() != defOf(t, b, "opencode").ExecutionFingerprint() {
		t.Fatal("cosmetic metadata changed the execution fingerprint")
	}
	if defOf(t, a, "opencode").ExecutionFingerprint() != defOf(t, catBuild(t, ""), "opencode").ExecutionFingerprint() {
		t.Fatal("fingerprint not stable across builds")
	}
	if defOf(t, a, "opencode").ExecutionFingerprint() == defOf(t, catBuild(t, "schema_version=1\n[[harness]]\nid=\"opencode\"\ninterpose_commands=false\n"), "opencode").ExecutionFingerprint() {
		t.Fatal("material change did not change the fingerprint")
	}
}

func TestConfigRootSyntaxRejects(t *testing.T) {
	for _, p := range []string{"", ".", "./", "..", "../x", "/etc", "\\windows", "C:evil", "a/../b"} {
		if err := validateRootSyntax(p); err == nil {
			t.Fatalf("validateRootSyntax(%q) accepted", p)
		}
	}
	for _, p := range []string{".config/agent", ".myagent", ".opencode/bin", "sub/dir"} {
		if err := validateRootSyntax(p); err != nil {
			t.Fatalf("validateRootSyntax(%q) rejected: %v", p, err)
		}
	}
}

func TestConfigRootUserPolicy(t *testing.T) {
	reject := []string{".", "./", ".ssh", ".aws", ".gnupg", ".kube", ".config", ".local", ".cache", "Downloads", "Documents/notes", "..", "/etc"}
	for _, p := range reject {
		if err := validateRootPolicy(p, true); err == nil {
			t.Fatalf("user root %q accepted", p)
		}
	}
	accept := []string{".myagent", ".opencode/bin", ".config/myagent", ".local/share/myagent", ".local/state/myagent", ".cache/myagent"}
	for _, p := range accept {
		if err := validateRootPolicy(p, true); err != nil {
			t.Fatalf("user root %q rejected: %v", p, err)
		}
	}
	// Shipped roots keep their narrower trusted policy.
	if err := validateRootPolicy(".opencode", false); err != nil {
		t.Fatalf("shipped root rejected: %v", err)
	}
}

func TestResolveRootSafeSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "cfg")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRootSafe(home, "cfg"); err == nil {
		t.Fatal("symlink to outside home was accepted")
	}
	if err := os.Symlink("/", filepath.Join(home, "root")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRootSafe(home, "root"); err == nil {
		t.Fatal("symlink to / was accepted")
	}
	// Nested symlink escape.
	if err := os.MkdirAll(filepath.Join(home, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "a", "b")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRootSafe(home, "a/b"); err == nil {
		t.Fatal("nested symlink escape was accepted")
	}
	// Symlink that stays inside home is allowed and resolved.
	real := filepath.Join(home, "realcfg")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(home, "linkcfg")); err != nil {
		t.Fatal(err)
	}
	got, err := resolveRootSafe(home, "linkcfg")
	if err != nil {
		t.Fatalf("in-home symlink rejected: %v", err)
	}
	if got != real {
		t.Fatalf("resolved = %q, want %q", got, real)
	}
	// Symlink onto a sensitive location is rejected.
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, ".ssh"), filepath.Join(home, "sshlink")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRootSafe(home, "sshlink"); err == nil {
		t.Fatal("symlink onto .ssh was accepted")
	}
}

func TestWellKnownSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "opencode"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, ".wk")); err != nil {
		t.Fatal(err)
	}
	if got := expandHomePattern(home, ".wk"); len(got) != 0 {
		t.Fatalf("escaping well_known search root was granted: %v", got)
	}
	// A real in-home well_known dir is granted.
	real := filepath.Join(home, ".realwk")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	got := expandHomePattern(home, ".realwk")
	if len(got) != 1 || got[0] != real {
		t.Fatalf("in-home well_known dir = %v, want [%s]", got, real)
	}
}

func TestCatalogBounds(t *testing.T) {
	// Too many definitions.
	var b strings.Builder
	b.WriteString("schema_version=1\n")
	for i := 0; i < maxCatalogDefinitions+1; i++ {
		b.WriteString("[[harness]]\nid=\"h")
		b.WriteString(itoa(i))
		b.WriteString("\"\nexecutables=[\"h")
		b.WriteString(itoa(i))
		b.WriteString("\"]\n")
	}
	if _, err := buildCatalog(shippedCatalogTOML, b.String()); err == nil {
		t.Fatal("definition count limit not enforced")
	}
	// Too many list entries.
	var c strings.Builder
	c.WriteString("schema_version=1\n[[harness]]\nid=\"big\"\nexecutables=[")
	for i := 0; i <= maxListEntries; i++ {
		if i > 0 {
			c.WriteString(",")
		}
		c.WriteString("\"e")
		c.WriteString(itoa(i))
		c.WriteString("\"")
	}
	c.WriteString("]\n")
	cat, err := buildCatalog(shippedCatalogTOML, c.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.ByID("big"); ok {
		t.Fatal("list entry limit not enforced")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
