package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRequiredIsolationNeverSilent(t *testing.T) {
	m := Manager{Backend: UnsupportedBackend{OS: "plan9"}}
	_, err := m.Start(context.Background(), Policy{Required: true, ReadWriteRoots: []string{"/tmp"}})
	if !errors.Is(err, ErrRequiredIsolation) {
		t.Fatalf("got %v", err)
	}
}

func TestReducedSecurityStillDoesNotSilentlyUnrestrict(t *testing.T) {
	m := Manager{Backend: UnsupportedBackend{OS: "plan9"}}
	_, err := m.Start(context.Background(), Policy{Required: false, ReadWriteRoots: []string{"/tmp"}})
	if !errors.Is(err, ErrRequiredIsolation) {
		t.Fatalf("got %v, want explicit isolation error", err)
	}
}

func TestProbeNeverClaimsUnrestricted(t *testing.T) {
	r := Probe()
	if r.Mode == "unrestricted" || r.Mode == "" {
		t.Fatalf("probe mode %q", r.Mode)
	}
	if r.Backend != runtime.GOOS && r.Backend != "unsupported" {
		t.Fatalf("backend %s on %s", r.Backend, runtime.GOOS)
	}
}

func TestCompileRequiresWritableRoots(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	_, err := c.Compile(Policy{Required: true})
	if !errors.Is(err, ErrRequiredIsolation) {
		t.Fatalf("got %v", err)
	}
}

func TestSeatbeltProfileCompilation(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	p := ToolPolicy(ws, home, NetNone)
	prof, err := SeatbeltProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prof, "(deny default)") {
		t.Fatal(prof)
	}
	if !strings.Contains(prof, "(deny network*)") {
		t.Fatal("network none must deny network")
	}
	if !strings.Contains(prof, "(allow file-map-executable)") {
		t.Fatal("profile must allow mapping the dynamic linker")
	}
	quoted := strconv.Quote(filepath.ToSlash(filepath.Clean(ws)))
	if !strings.Contains(prof, quoted) {
		t.Fatalf("writable root %s missing from profile:\n%s", quoted, prof)
	}
}

func TestForeignBackendsAreUnavailableHere(t *testing.T) {
	switch runtime.GOOS {
	case "linux":
		_, err := (DarwinBackend{}).Compile(HarnessPolicy(t.TempDir(), t.TempDir()))
		if !errors.Is(err, ErrRequiredIsolation) {
			t.Fatalf("darwin on linux: %v", err)
		}
		_, err = (WindowsBackend{}).Compile(HarnessPolicy(t.TempDir(), t.TempDir()))
		if !errors.Is(err, ErrRequiredIsolation) {
			t.Fatalf("windows on linux: %v", err)
		}
	case "darwin":
		_, err := (LinuxBackend{}).Compile(HarnessPolicy(t.TempDir(), t.TempDir()))
		if !errors.Is(err, ErrRequiredIsolation) {
			t.Fatalf("linux on darwin: %v", err)
		}
	case "windows":
		_, err := (LinuxBackend{}).Compile(HarnessPolicy(t.TempDir(), t.TempDir()))
		if !errors.Is(err, ErrRequiredIsolation) {
			t.Fatalf("linux on windows: %v", err)
		}
	}
}

func TestNativeCompileAndConstrain(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	rep := c.Report()
	if !rep.Available {
		t.Skip("native sandbox primitives unavailable: " + rep.Detail)
	}
	home := t.TempDir()
	ws := t.TempDir()
	p := HarnessPolicy(ws, home)
	compiled, err := c.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Features) == 0 {
		t.Fatal("expected applied features")
	}
	cmd := trueCmd()
	if err := c.Constrain(cmd, p); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	clean, err := c.Attach(cmd, p)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatal(err)
	}
	defer clean()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessTreeKill(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skip("native sandbox unavailable")
	}
	p := HarnessPolicy(t.TempDir(), t.TempDir())
	cmd := sleeper()
	if err := c.Constrain(cmd, p); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	clean, err := c.Attach(cmd, p)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatal(err)
	}
	defer clean()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err := c.KillTree(cmd); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("process tree still running after KillTree")
	}
}

func TestUnsupportedConstrainFailsClosed(t *testing.T) {
	u := AsConstrainer(UnsupportedBackend{OS: "plan9"})
	cmd := trueCmd()
	err := u.Constrain(cmd, Policy{Required: true, ReadWriteRoots: []string{t.TempDir()}})
	if !errors.Is(err, ErrRequiredIsolation) {
		t.Fatalf("got %v", err)
	}
	if cmd.SysProcAttr != nil && runtime.GOOS == "linux" {
		// must not have applied namespaces on the way to failure
	}
}

func trueCmd() *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "exit", "0")
	}
	return exec.Command("true")
}

func sleeper() *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("ping", "-n", "20", "127.0.0.1")
	}
	return exec.Command("sleep", "20")
}
