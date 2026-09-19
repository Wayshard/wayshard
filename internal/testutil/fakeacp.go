package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ExeName returns a platform-correct executable file name.
func ExeName(base string) string {
	base = strings.TrimSuffix(base, ".exe")
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// ModuleRoot walks up from the working directory to go.mod.
func ModuleRoot(t testing.TB) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			t.Fatal("go.mod not found")
		}
	}
}

// BuildFakeACP compiles cmd/wayshard-fake-acp into a temporary executable.
func BuildFakeACP(t testing.TB) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), ExeName("wayshard-fake-acp"))
	cmd := exec.Command("go", "build", "-o", out, "./cmd/wayshard-fake-acp")
	cmd.Dir = ModuleRoot(t)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build wayshard-fake-acp: %v\n%s", err, b)
	}
	return out
}
