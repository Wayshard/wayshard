//go:build linux

package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
)

func buildUser(t *testing.T, body string) *Catalog {
	t.Helper()
	cat, err := buildCatalog(shippedCatalogTOML, body)
	if err != nil {
		t.Fatalf("buildCatalog: %v", err)
	}
	return cat
}

func TestShippedCatalogParses(t *testing.T) {
	cat, err := buildCatalog(shippedCatalogTOML, "")
	if err != nil {
		t.Fatal(err)
	}
	if cat.SchemaVersion != CatalogSchemaVersion {
		t.Fatalf("schema version = %d", cat.SchemaVersion)
	}
	if len(cat.Diagnostics) != 0 {
		t.Fatalf("unexpected shipped diagnostics: %v", cat.Diagnostics)
	}
	oc, ok := cat.ByID("opencode")
	if !ok || !oc.Enabled || oc.Source != SourceShipped {
		t.Fatalf("opencode = %+v ok=%v", oc, ok)
	}
}

func TestUserOverrideFields(t *testing.T) {
	cat := buildUser(t, `
schema_version = 1
[[harness]]
id = "opencode"
acp_args = ["acp", "--custom"]
display_name = "OpenCode Custom"
`)
	oc, ok := cat.ByID("opencode")
	if !ok {
		t.Fatal("opencode missing")
	}
	if oc.Source != SourceOverridden {
		t.Fatalf("source = %s", oc.Source)
	}
	if strings.Join(oc.ACPArgs, " ") != "acp --custom" {
		t.Fatalf("acp_args = %v", oc.ACPArgs)
	}
	if oc.DisplayName != "OpenCode Custom" {
		t.Fatalf("display_name = %q", oc.DisplayName)
	}
	// Unspecified fields inherit from the shipped definition.
	if !oc.InterposeCommands || oc.ModelSelection != "config_option" {
		t.Fatalf("inherited fields lost: %+v", oc)
	}
}

func TestUserDisableShippedAndDeleteRestores(t *testing.T) {
	cat := buildUser(t, `
schema_version = 1
[[harness]]
id = "opencode"
enabled = false
`)
	oc, _ := cat.ByID("opencode")
	if oc.Enabled {
		t.Fatal("opencode should be disabled")
	}
	if oc.Source != SourceOverridden {
		t.Fatalf("source = %s", oc.Source)
	}
	// Deleting the override restores the shipped definition.
	restored, _ := buildCatalog(shippedCatalogTOML, "")
	roc, _ := restored.ByID("opencode")
	if !roc.Enabled || roc.Source != SourceShipped {
		t.Fatalf("delete did not restore shipped: %+v", roc)
	}
}

func TestUserCustomDefinition(t *testing.T) {
	cat := buildUser(t, `
schema_version = 1
[[harness]]
id = "my-custom-agent"
display_name = "My Custom Agent"
executables = ["my-custom-agent"]
acp = "native"
acp_args = ["acp"]
`)
	d, ok := cat.ByID("my-custom-agent")
	if !ok || d.Source != SourceUser {
		t.Fatalf("custom definition = %+v ok=%v", d, ok)
	}
	// It is absent without the user file.
	plain, _ := buildCatalog(shippedCatalogTOML, "")
	if _, ok := plain.ByID("my-custom-agent"); ok {
		t.Fatal("custom definition leaked without user file")
	}
}

func TestDuplicateUserIDRejected(t *testing.T) {
	cat := buildUser(t, `
schema_version = 1
[[harness]]
id = "dup"
executables = ["dup-a"]
[[harness]]
id = "dup"
executables = ["dup-b"]
`)
	count := 0
	for _, d := range cat.Definitions {
		if d.ID == "dup" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate id kept %d definitions", count)
	}
	found := false
	for _, dg := range cat.Diagnostics {
		if dg.ID == "dup" && strings.Contains(dg.Message, "duplicate") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no duplicate diagnostic: %v", cat.Diagnostics)
	}
}

func TestMalformedTOMLDiagnosed(t *testing.T) {
	if _, err := buildCatalog(shippedCatalogTOML, "schema_version = 1\n[[harness]\n"); err == nil {
		t.Fatal("malformed TOML not diagnosed")
	}
}

func TestUnsupportedSchemaVersionRejected(t *testing.T) {
	if _, err := buildCatalog(shippedCatalogTOML, "schema_version = 99\n"); err == nil {
		t.Fatal("unsupported schema version not rejected")
	}
	if _, err := buildCatalog(shippedCatalogTOML, "[[harness]]\nid = \"x\"\n"); err == nil {
		t.Fatal("missing schema_version not rejected")
	}
}

func TestInvalidEntryIsolatedFromValid(t *testing.T) {
	cat := buildUser(t, `
schema_version = 1
[[harness]]
id = "bad"
executables = ["bad"]
config_roots = ["/etc"]
[[harness]]
id = "good"
executables = ["good"]
acp = "native"
`)
	if _, ok := cat.ByID("bad"); ok {
		t.Fatal("security-invalid entry was accepted")
	}
	if _, ok := cat.ByID("good"); !ok {
		t.Fatalf("valid entry was destroyed by an invalid one: %v", cat.Diagnostics)
	}
}

func TestInvalidDeclarationsFailClosed(t *testing.T) {
	cases := map[string]string{
		"absolute well_known":  `well_known = ["/usr/bin"]`,
		"traversal well_known": `well_known = ["../secret"]`,
		"path executable":      `executables = ["/usr/bin/evil"]`,
		"package runner":       `executables = ["npx"]`,
		"unknown field":        `sandbox = false`,
		"bad acp":              `acp = "unrestricted"`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			cat := buildUser(t, "schema_version = 1\n[[harness]]\nid = \"evil\"\nexecutables = [\"evil\"]\n"+body+"\n")
			if _, ok := cat.ByID("evil"); ok {
				t.Fatalf("invalid declaration accepted: %s", body)
			}
		})
	}
}

// TestCustomHarnessDiscovery is the catalog extensibility proof: a harness known
// only to a temporary user catalog is discovered and ACP-probed without any Go
// change for its id.
func TestCustomHarnessDiscovery(t *testing.T) {
	if fakeACP == "" {
		t.Skip("fake harness unavailable")
	}
	binDir := t.TempDir()
	exe := filepath.Join(binDir, "my-custom-agent")
	if err := os.Symlink(fakeACP, exe); err != nil {
		b, err := os.ReadFile(fakeACP)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(exe, b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	userPath := filepath.Join(t.TempDir(), "harnesses.toml")
	if err := os.WriteFile(userPath, []byte(`
schema_version = 1
[[harness]]
id = "my-custom-agent"
display_name = "My Custom Agent"
executables = ["my-custom-agent"]
version_args = ["--version"]
acp = "native"
acp_args = []
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got, err := Discover(ctx, DiscoverOptions{
		PATH: binDir, Home: t.TempDir(),
		IncludeLoginPATH: false, WellKnownDirs: false,
		Probe: true, ProbeTimeout: 8 * time.Second,
		CatalogPath: userPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	inst := findInstallation(t, got, exe)
	if inst.DefinitionID != "my-custom-agent" || inst.DefinitionSource != SourceUser {
		t.Fatalf("custom definition not used: %+v", inst)
	}
	if inst.Health != domain.HarnessReady || inst.ACPStatus != "ok" {
		t.Fatalf("custom harness not ACP-ready: health=%s acp=%s notes=%v", inst.Health, inst.ACPStatus, inst.Notes)
	}
}

// findInstallation returns the discovered installation for exe, failing the test
// if it is absent.
func findInstallation(t *testing.T, list []Installation, exe string) Installation {
	t.Helper()
	for _, inst := range list {
		if inst.Executable == exe {
			return inst
		}
	}
	t.Fatalf("installation %s not found in %+v", exe, list)
	return Installation{}
}
