package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wayshard/wayshard/internal/harness"
)

// TestMain pins a deterministic harness catalog so app tests exercise only the
// bundled fake ACP harness regardless of which real coding harnesses happen to
// be installed on the machine running the tests. Wayshard now runs discovered
// harnesses as the server OS user (no provider/sandbox gating), so ambient
// harnesses would otherwise leak into routing.
func TestMain(m *testing.M) {
	cat := harness.ShippedCatalog()
	var b strings.Builder
	b.WriteString("schema_version = 1\n")
	for _, d := range cat.Definitions {
		if d.ID == "wayshard-fake-acp" {
			continue
		}
		fmt.Fprintf(&b, "[[harness]]\nid = %q\nenabled = false\n\n", d.ID)
	}
	dir, err := os.MkdirTemp("", "wayshard-app-test-catalog-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "test catalog:", err)
		os.Exit(1)
	}
	path := filepath.Join(dir, "harnesses.toml")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "test catalog:", err)
		os.Exit(1)
	}
	os.Setenv("WAYSHARD_HARNESS_CATALOG", path)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
