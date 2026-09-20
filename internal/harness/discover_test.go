package harness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/acp"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/testutil"
)

var fakeACP string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wayshard-fake-acp-discover")
	if err != nil {
		panic(err)
	}
	fakeACP = filepath.Join(dir, testutil.ExeName("wayshard-fake-acp"))
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	cmd := exec.Command("go", "build", "-o", fakeACP, "./cmd/wayshard-fake-acp")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		panic("build wayshard-fake-acp: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func placeFake(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, testutil.ExeName(name))
	if err := os.Symlink(fakeACP, dst); err != nil {
		b, err2 := os.ReadFile(fakeACP)
		if err2 != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

func TestDiscoverPATHReady(t *testing.T) {
	dir := t.TempDir()
	placeFake(t, dir, "wayshard-fake-acp")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	got, err := Discover(ctx, DiscoverOptions{
		PATH:             dir,
		IncludeLoginPATH: false,
		WellKnownDirs:    false,
		Probe:            true,
		ProbeTimeout:     8 * time.Second,
		LookupNames:      []string{"wayshard-fake-acp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d installs: %+v", len(got), got)
	}
	if got[0].Health != domain.HarnessReady {
		t.Fatalf("health = %s notes=%v", got[0].Health, got[0].Notes)
	}
	if got[0].Adapter != AdapterGeneric {
		t.Fatalf("adapter = %s", got[0].Adapter)
	}
	if got[0].Compatibility != domain.CompatRoutable && got[0].Compatibility != domain.CompatEnhanced {
		t.Fatalf("compat = %s", got[0].Compatibility)
	}
	if got[0].Isolation != domain.IsolationOuterOnly {
		t.Fatalf("isolation = %s", got[0].Isolation)
	}
	if got[0].Resume != domain.ResumeReconstruct {
		t.Fatalf("resume = %s (must not assume native resume)", got[0].Resume)
	}
}

func TestDiscoverWellKnownDir(t *testing.T) {
	home := t.TempDir()
	placeFake(t, filepath.Join(home, ".local", "bin"), "wayshard-fake-acp")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	got, err := Discover(ctx, DiscoverOptions{
		PATH:             filepath.Join(home, "empty-path"),
		Home:             home,
		IncludeLoginPATH: false,
		WellKnownDirs:    true,
		Probe:            true,
		LookupNames:      []string{"wayshard-fake-acp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
}

func TestDiscoverExplicitPath(t *testing.T) {
	dir := t.TempDir()
	exe := placeFake(t, dir, "custom-agent")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	got, err := Discover(ctx, DiscoverOptions{
		PATH:             filepath.Join(dir, "unused"),
		IncludeLoginPATH: false,
		WellKnownDirs:    false,
		Probe:            true,
		LookupNames:      []string{"no-such-harness"},
		ExtraPaths:       []string{exe},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Health != domain.HarnessReady {
		t.Fatalf("%+v", got)
	}
}

func TestDiscoverNeverRunsNpx(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "npx-ran")
	npx := filepath.Join(dir, "npx")
	script := "#!/bin/sh\necho ran > '" + sentinel + "'\nexit 0\n"
	if err := os.WriteFile(npx, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	placeFake(t, dir, "wayshard-fake-acp")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	got, err := Discover(ctx, DiscoverOptions{
		PATH:             dir,
		IncludeLoginPATH: false,
		WellKnownDirs:    false,
		Probe:            true,
		LookupNames:      []string{"npx", "wayshard-fake-acp"},
		ExtraPaths:       []string{npx},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("npx was executed; Wayshard must never install via package runners")
	}
	var sawNpxRefuse, sawFake bool
	for _, in := range got {
		if strings.Contains(strings.Join(in.Notes, " "), "package-runner") {
			sawNpxRefuse = true
		}
		if normalizeExecName(in.Executable) == "wayshard-fake-acp" {
			sawFake = true
		}
		if normalizeExecName(in.Executable) == "npx" && in.Health == domain.HarnessReady {
			t.Fatal("npx must not be treated as a ready harness")
		}
	}
	if !sawNpxRefuse || !sawFake {
		t.Fatalf("refuse=%v fake=%v installs=%+v", sawNpxRefuse, sawFake, got)
	}
}

func TestDiscoverAuthAndMalformed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	t.Setenv("WAYSHARD_FAKE_SCENARIO", "auth_required")
	dir := t.TempDir()
	placeFake(t, dir, "wayshard-fake-acp")
	got, err := Discover(ctx, DiscoverOptions{
		PATH: dir, IncludeLoginPATH: false, WellKnownDirs: false, Probe: true,
		LookupNames: []string{"wayshard-fake-acp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Health != domain.HarnessReady || got[0].AuthStatus != "unknown" {
		t.Fatalf("advertised-auth should be ready/unknown, health=%v auth=%v %+v", got[0].Health, got[0].AuthStatus, got)
	}

	t.Setenv("WAYSHARD_FAKE_SCENARIO", "malformed")
	dir2 := t.TempDir()
	placeFake(t, dir2, "wayshard-fake-acp")
	got, err = Discover(ctx, DiscoverOptions{
		PATH: dir2, IncludeLoginPATH: false, WellKnownDirs: false, Probe: true,
		LookupNames: []string{"wayshard-fake-acp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Health != domain.HarnessIncompatible {
		t.Fatalf("malformed health=%v %+v", got[0].Health, got)
	}
}

func TestDiscoverMissingIsEmpty(t *testing.T) {
	ctx := context.Background()
	got, err := Discover(ctx, DiscoverOptions{
		PATH:             t.TempDir(),
		IncludeLoginPATH: false,
		WellKnownDirs:    false,
		Probe:            true,
		LookupNames:      []string{"opencode", "codex"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no installs, got %+v", got)
	}
}

func TestDiscoverConfiguredMissingPath(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "not-installed")
	got, err := Discover(ctx, DiscoverOptions{
		PATH:             t.TempDir(),
		IncludeLoginPATH: false,
		WellKnownDirs:    false,
		Probe:            false,
		ExtraPaths:       []string{p},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Health != domain.HarnessUnavailable {
		t.Fatalf("%+v", got)
	}
}

func TestAdaptersDoNotAssumeOptionalCaps(t *testing.T) {
	empty := acp.AgentCapabilities{}
	oc := OpenCodeAdapter{}
	if oc.SessionResume(empty) != domain.ResumeReconstruct {
		t.Fatal("must not assume native resume from the OpenCode name")
	}
	if oc.Isolation(empty) != domain.IsolationAdapterBridge {
		t.Fatal("OpenCode adapter reports adapter_bridge via command interposition")
	}
	if oc.LaunchSpec(Installation{Executable: "/usr/bin/opencode"}).Args[0] != "acp" {
		t.Fatal("opencode launch must use acp subcommand")
	}
	cx := CodexAdapter{}
	if cx.SessionResume(empty) != domain.ResumeReconstruct {
		t.Fatal("must not assume native resume from the Codex name")
	}
	g := GenericACPAdapter{}
	if g.Isolation(empty) != domain.IsolationOuterOnly {
		t.Fatal("generic is outer_only")
	}
	if g.InterposeCommands() {
		t.Fatal("generic does not interpose")
	}
	native := acp.AgentCapabilities{LoadSession: true}
	if oc.SessionResume(native) != domain.ResumeNative {
		t.Fatal("native resume only when advertised")
	}
}

func TestAdapterForName(t *testing.T) {
	if AdapterFor("", "/opt/opencode").ID() != AdapterOpenCode {
		t.Fatal("opencode")
	}
	if AdapterFor("", "codex-acp").ID() != AdapterCodex {
		t.Fatal("codex")
	}
	if AdapterFor("", fakeACP).ID() != AdapterGeneric {
		t.Fatal("fake is generic")
	}
}

func TestForbiddenLauncher(t *testing.T) {
	for _, n := range []string{"npx", "/usr/bin/npm", "bunx.exe"} {
		if !forbiddenLauncher(n) {
			t.Fatalf("%s should be forbidden", n)
		}
	}
	if forbiddenLauncher("opencode") {
		t.Fatal("opencode is not a package runner")
	}
}
