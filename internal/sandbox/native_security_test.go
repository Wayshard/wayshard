//go:build darwin || windows

// Native sandbox tests for platforms where Wayshard cannot establish a
// non-removable process-tree ownership boundary.
//
// macOS cannot: Seatbelt confines the process, but an untrusted process controls
// its children's environment (so an ownership token can be stripped) and can
// setsid out of the process group. Windows cannot: Job Objects are process/
// resource management only. On both, a required policy must fail closed BEFORE
// any untrusted code runs. These tests prove that, rather than exercising
// execution that must never happen.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const helperEnv = "GO_WANT_SANDBOX_HELPER"

// TestSandboxHelperProcess is the fixture entry point. It is inert unless the
// helper env var is set, so it is harmless during a normal test run.
func TestSandboxHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("not a sandbox helper invocation")
	}
	mode := os.Getenv("SANDBOX_HELPER_MODE")
	arg := os.Getenv("SANDBOX_HELPER_ARG")
	switch mode {
	case "marker":
		_ = os.WriteFile(arg, []byte("ran"), 0o600)
		os.Exit(0)
	case "env":
		fmt.Printf("HOME=%s\nUSERPROFILE=%s\nTMPDIR=%s\nTEMP=%s\nTMP=%s\n", os.Getenv("HOME"), os.Getenv("USERPROFILE"), os.Getenv("TMPDIR"), os.Getenv("TEMP"), os.Getenv("TMP"))
		if v := os.Getenv("TYPESAFE_API_KEY"); v != "" {
			fmt.Printf("LEAK_TYPESAFE=%s\n", v)
		}
		if v := os.Getenv("WAYSHARD_VAULT_KEY"); v != "" {
			fmt.Printf("LEAK_VAULT=%s\n", v)
		}
		os.Exit(0)
	case "spawn":
		n, _ := strconv.Atoi(arg)
		pids := []int{}
		for i := 0; i < n; i++ {
			c := exec.Command(os.Args[0], "-test.run=TestSandboxHelperProcess")
			c.Env = append(os.Environ(), helperEnv+"=1", "SANDBOX_HELPER_MODE=sleep")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Start(); err != nil {
				fmt.Printf("SPAWN_ERR=%v\n", err)
				os.Exit(4)
			}
			pids = append(pids, c.Process.Pid)
		}
		for _, p := range pids {
			fmt.Printf("PID=%d\n", p)
		}
		fmt.Println("SPAWNED")
		os.Exit(0)
	case "sleep":
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	default:
		fmt.Printf("unknown mode %q\n", mode)
		os.Exit(2)
	}
}

func helperExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return exe
}

func helperEnvFor(mode, arg string, extra []string) []string {
	env := append([]string{}, extra...)
	env = append(env,
		helperEnv+"=1",
		"SANDBOX_HELPER_MODE="+mode,
		"SANDBOX_HELPER_ARG="+arg,
	)
	return env
}

// TestNativeFailClosedBeforeExec proves that when the required policy cannot be
// compiled, no untrusted code runs: the marker file is never created.
func TestNativeFailClosedBeforeExec(t *testing.T) {
	ws := t.TempDir()
	home := t.TempDir()
	marker := filepath.Join(ws, "executed.marker")
	// Force an uncompilable required policy: an unsupported network mode.
	p := ToolPolicy(ws, home, NetNone)
	p.ReadOnlyRoots = append(p.ReadOnlyRoots, filepath.Dir(helperExe(t)))
	p.Network = NetBrokered
	env := helperEnvFor("marker", marker, ToolEnv(home, home, nil))
	_, err := RunConstrainedOutput(context.Background(), p, 10*time.Second, 64<<10, helperExe(t), []string{"-test.run=TestSandboxHelperProcess"}, env)
	if err == nil {
		t.Fatalf("uncompilable required policy must fail closed")
	}
	if !errors.Is(err, ErrRequiredIsolation) {
		t.Fatalf("want ErrRequiredIsolation, got %v", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatalf("target executed before containment was established")
	}
}

// TestNativeRequiredPoliciesFailBeforeExec is the mandatory acceptance test for
// platforms without a non-removable ownership boundary: every required
// production policy must refuse before any target code runs.
func TestNativeRequiredPoliciesFailBeforeExec(t *testing.T) {
	ws := t.TempDir()
	home := t.TempDir()
	exe := helperExe(t)
	policies := []struct {
		name string
		pol  Policy
	}{
		{"tool", ToolPolicy(ws, home, NetNone)},
		{"harness", HarnessPolicy(ws, home)},
		{"probe", ProbePolicy(exe, home, home)},
		{"readonly_view", ReadOnlyViewPolicy(ws, home)},
	}
	for _, tc := range policies {
		t.Run(tc.name, func(t *testing.T) {
			marker := filepath.Join(ws, tc.name+".marker")
			env := helperEnvFor("marker", marker, ToolEnv(home, home, nil))
			_, err := RunConstrainedOutput(context.Background(), tc.pol, 10*time.Second, 64<<10, exe, []string{"-test.run=TestSandboxHelperProcess"}, env)
			if err == nil {
				t.Fatalf("%s: required policy must fail closed", tc.name)
			}
			if !errors.Is(err, ErrRequiredIsolation) {
				t.Fatalf("%s: want ErrRequiredIsolation, got %v", tc.name, err)
			}
			if _, statErr := os.Stat(marker); statErr == nil {
				t.Fatalf("%s: target executed before containment was established", tc.name)
			}
		})
	}
}

// TestWindowsJobObjectManagesNonRequiredProcessTree exercises the Windows Job
// Object management path (used for resource/process management, not as a
// security sandbox) with a non-required policy, proving the tree is terminated.
func TestWindowsJobObjectManagesNonRequiredProcessTree(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows job object management")
	}
	ws := t.TempDir()
	home := t.TempDir()
	p := ToolPolicy(ws, home, NetNone)
	p.Required = false
	p.Network = ""
	p.ReadOnlyRoots = append(p.ReadOnlyRoots, filepath.Dir(helperExe(t)))

	c := AsConstrainer(WindowsBackend{})
	if _, err := c.Compile(p); err != nil {
		t.Skipf("job object management unavailable: %v", err)
	}
	cmd := exec.Command(helperExe(t), "-test.run=TestSandboxHelperProcess")
	cmd.Env = helperEnvFor("spawn", "2", ToolEnv(home, home, nil))
	var stdout strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	if err := c.Constrain(cmd, p); err != nil {
		t.Fatalf("constrain: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	clean, err := c.Attach(cmd, p)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("attach: %v", err)
	}
	defer clean()
	pids := waitForPIDs(t, &stdout, 2)
	if err := c.KillTree(cmd); err != nil {
		t.Fatalf("kill tree: %v", err)
	}
	_ = cmd.Wait()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		alive := false
		for _, pid := range pids {
			if processAlive(pid) {
				alive = true
				break
			}
		}
		if !alive {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("job object did not terminate descendants: %v", pids)
}

// TestWindowsSyntheticEnvNonRequired proves the Windows env filtering and
// synthetic HOME/USERPROFILE/TEMP/TMP on the management-only (non-required)
// path, since required Windows policies fail closed.
func TestWindowsSyntheticEnvNonRequired(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows synthetic environment")
	}
	ws := t.TempDir()
	home := t.TempDir()
	p := ToolPolicy(ws, home, NetNone)
	p.Required = false
	p.Network = ""
	p.ReadOnlyRoots = append(p.ReadOnlyRoots, filepath.Dir(helperExe(t)))
	env := helperEnvFor("env", "", ToolEnv(home, home, nil))
	env = append(env, "TYPESAFE_API_KEY=should-not-leak", "WAYSHARD_VAULT_KEY=should-not-leak")
	out, err := RunConstrainedOutput(context.Background(), p, 20*time.Second, 64<<10, helperExe(t), []string{"-test.run=TestSandboxHelperProcess"}, env)
	if err != nil {
		t.Fatalf("env helper: %v (out=%s)", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "HOME="+home) || !strings.Contains(text, "USERPROFILE="+home) {
		t.Fatalf("synthetic HOME/USERPROFILE missing: %s", text)
	}
	if !strings.Contains(text, "TEMP="+home) || !strings.Contains(text, "TMP="+home) {
		t.Fatalf("synthetic TEMP/TMP missing: %s", text)
	}
	if strings.Contains(text, "LEAK_TYPESAFE") || strings.Contains(text, "LEAK_VAULT") {
		t.Fatalf("sensitive ambient env leaked into the process: %s", text)
	}
}

func waitForPIDs(t *testing.T, out *strings.Builder, want int) []int {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pids := parsePIDs(out.String())
		if len(pids) >= want {
			return pids
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("fixture did not report %d child pids; output:\n%s", want, out.String())
	return nil
}

func parsePIDs(text string) []int {
	var out []int
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "PID=") {
			if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PID="))); err == nil {
				out = append(out, n)
			}
		}
	}
	return out
}

func processAlive(pid int) bool {
	return platformProcessAlive(pid)
}
