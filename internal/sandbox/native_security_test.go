//go:build darwin || windows

// Native sandbox security tests. They run real processes through the production
// Constrainer on macOS (sandbox-exec/Seatbelt) and Windows (Job Object
// management / honest fail-closed), and prove the actual behaviour rather than
// profile strings.
//
// The fixture is the test binary itself re-executed with GO_WANT_SANDBOX_HELPER=1
// and a mode, so no separate helper binary has to be built or installed.
package sandbox

import (
	"context"
	"fmt"
	"net"
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
		fmt.Printf("HOME=%s\nTMPDIR=%s\nTEMP=%s\nTMP=%s\n", os.Getenv("HOME"), os.Getenv("TMPDIR"), os.Getenv("TEMP"), os.Getenv("TMP"))
		if v := os.Getenv("TYPESAFE_API_KEY"); v != "" {
			fmt.Printf("LEAK_TYPESAFE=%s\n", v)
		}
		if v := os.Getenv("WAYSHARD_VAULT_KEY"); v != "" {
			fmt.Printf("LEAK_VAULT=%s\n", v)
		}
		os.Exit(0)
	case "read":
		if _, err := os.ReadFile(arg); err != nil {
			fmt.Println("DENIED")
			os.Exit(3)
		}
		fmt.Println("READ_OK")
		os.Exit(0)
	case "write":
		if err := os.WriteFile(arg, []byte("x"), 0o600); err != nil {
			fmt.Println("DENIED")
			os.Exit(3)
		}
		fmt.Println("WRITE_OK")
		os.Exit(0)
	case "connect":
		conn, err := net.DialTimeout("tcp", arg, 2*time.Second)
		if err != nil {
			fmt.Println("DENIED")
			os.Exit(3)
		}
		_ = conn.Close()
		fmt.Println("CONNECTED")
		os.Exit(0)
	case "listen":
		ln, err := net.Listen("tcp", arg)
		if err != nil {
			fmt.Println("DENIED")
			os.Exit(3)
		}
		_ = ln.Close()
		fmt.Println("LISTENING")
		os.Exit(0)
	case "spawn":
		// Spawn `arg` nested sleeping helpers and print their PIDs, then sleep so
		// the parent can kill the whole tree.
		n, _ := strconv.Atoi(arg)
		pids := []int{}
		for i := 0; i < n; i++ {
			c := exec.Command(os.Args[0], "-test.run=TestSandboxHelperProcess")
			c.Env = append(os.Environ(), helperEnv+"=1", "SANDBOX_HELPER_MODE=sleep")
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

// sandboxTestPolicy returns a required policy rooted at a workspace plus the
// test binary's own directory (so the fixture can exec).
func sandboxTestPolicy(t *testing.T, ws, home string) Policy {
	t.Helper()
	p := ToolPolicy(ws, home, NetNone)
	p.ReadOnlyRoots = append(p.ReadOnlyRoots, filepath.Dir(helperExe(t)))
	return p
}

// TestNativeFailClosedBeforeExec proves that when the required policy cannot be
// compiled, no untrusted code runs: the marker file is never created.
func TestNativeFailClosedBeforeExec(t *testing.T) {
	ws := t.TempDir()
	home := t.TempDir()
	marker := filepath.Join(ws, "executed.marker")
	// Force an uncompilable required policy: an unsupported network mode.
	p := sandboxTestPolicy(t, ws, home)
	p.Network = NetBrokered
	env := helperEnvFor("marker", marker, ToolEnv(home, home, nil))
	_, err := RunConstrainedOutput(context.Background(), p, 10*time.Second, 64<<10, helperExe(t), []string{"-test.run=TestSandboxHelperProcess"}, env)
	if err == nil {
		t.Fatalf("uncompilable required policy must fail closed")
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatalf("target executed before containment was established")
	}
}

// TestNativeEnvironmentIsSynthetic proves HOME/TEMP are scoped and sensitive
// ambient secrets do not leak, on whichever platform can enforce the policy.
func TestNativeEnvironmentIsSynthetic(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skipf("required isolation unavailable: %s", c.Report().Detail)
	}
	ws := t.TempDir()
	home := t.TempDir()
	t.Setenv("TYPESAFE_API_KEY", "should-not-leak")
	t.Setenv("WAYSHARD_VAULT_KEY", "should-not-leak")
	p := sandboxTestPolicy(t, ws, home)
	env := helperEnvFor("env", "", ToolEnv(home, home, nil))
	// ToolEnv strips ambient secrets; add them back explicitly to prove the
	// backend's filter also drops them.
	env = append(env, "TYPESAFE_API_KEY=should-not-leak", "WAYSHARD_VAULT_KEY=should-not-leak")
	out, err := RunConstrainedOutput(context.Background(), p, 20*time.Second, 64<<10, helperExe(t), []string{"-test.run=TestSandboxHelperProcess"}, env)
	if err != nil {
		t.Fatalf("env helper: %v (out=%s)", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "HOME="+home) {
		t.Fatalf("synthetic HOME missing: %s", text)
	}
	if runtime.GOOS == "darwin" && !strings.Contains(text, "TMPDIR="+home) {
		t.Fatalf("synthetic TMPDIR missing: %s", text)
	}
	if runtime.GOOS == "windows" {
		if !strings.Contains(text, "TEMP="+home) || !strings.Contains(text, "TMP="+home) {
			t.Fatalf("synthetic TEMP/TMP missing: %s", text)
		}
	}
	if strings.Contains(text, "LEAK_TYPESAFE") || strings.Contains(text, "LEAK_VAULT") {
		t.Fatalf("sensitive ambient env leaked into the sandbox: %s", text)
	}
}

// TestNativeFilesystemConfinement proves the read/write matrix under ToolPolicy.
func TestNativeFilesystemConfinement(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skipf("required isolation unavailable: %s", c.Report().Detail)
	}
	root := t.TempDir()
	ws := filepath.Join(root, "workspace")
	ro := filepath.Join(root, "readonly")
	secret := filepath.Join(root, "secret")
	home := filepath.Join(root, "home")
	for _, d := range []string{ws, ro, secret, home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(secret, "secret.txt"), []byte("s3cr3t"), 0o644)
	_ = os.WriteFile(filepath.Join(ro, "readable.txt"), []byte("ok"), 0o644)
	_ = os.WriteFile(filepath.Join(ws, "readable.txt"), []byte("ok"), 0o644)

	p := sandboxTestPolicy(t, ws, home)
	p.ReadOnlyRoots = append(p.ReadOnlyRoots, ro)
	p.ReadWriteRoots = append(p.ReadWriteRoots, ws, home)
	p.DeniedRoots = append(p.DeniedRoots, secret)

	run := func(mode, arg string) (string, error) {
		env := helperEnvFor(mode, arg, ToolEnv(home, home, nil))
		out, err := RunConstrainedOutput(context.Background(), p, 20*time.Second, 64<<10, helperExe(t), []string{"-test.run=TestSandboxHelperProcess"}, env)
		return string(out), err
	}

	if out, err := run("read", filepath.Join(ws, "readable.txt")); err != nil || !strings.Contains(out, "READ_OK") {
		t.Fatalf("workspace read must succeed: err=%v out=%s", err, out)
	}
	if out, err := run("write", filepath.Join(ws, "writable.txt")); err != nil || !strings.Contains(out, "WRITE_OK") {
		t.Fatalf("workspace write must succeed: err=%v out=%s", err, out)
	}
	if out, _ := run("read", filepath.Join(ro, "readable.txt")); !strings.Contains(out, "READ_OK") {
		t.Fatalf("read-only root read must succeed: %s", out)
	}
	if out, _ := run("write", filepath.Join(ro, "nope.txt")); strings.Contains(out, "WRITE_OK") {
		t.Fatalf("write into a read-only root must be denied: %s", out)
	}
	if out, _ := run("read", filepath.Join(secret, "secret.txt")); strings.Contains(out, "READ_OK") {
		t.Fatalf("host secret read must be denied: %s", out)
	}
	if out, _ := run("write", filepath.Join(secret, "nope.txt")); strings.Contains(out, "WRITE_OK") {
		t.Fatalf("host secret write must be denied: %s", out)
	}
}

// TestNativeNetworkNone proves NetworkNone denies connections to a host-side
// loopback listener (not merely DNS failure).
func TestNativeNetworkNone(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skipf("required isolation unavailable: %s", c.Report().Detail)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	go func() {
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	ws := t.TempDir()
	home := t.TempDir()
	p := sandboxTestPolicy(t, ws, home)
	env := helperEnvFor("connect", addr, ToolEnv(home, home, nil))
	out, _ := RunConstrainedOutput(context.Background(), p, 20*time.Second, 64<<10, helperExe(t), []string{"-test.run=TestSandboxHelperProcess"}, env)
	if strings.Contains(string(out), "CONNECTED") {
		t.Fatalf("NetworkNone must deny a loopback TCP connection to %s: %s", addr, out)
	}
}

// TestNativeProcessTreeKill proves cancellation kills the whole owned tree.
func TestNativeProcessTreeKill(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skipf("required isolation unavailable: %s", c.Report().Detail)
	}
	ws := t.TempDir()
	home := t.TempDir()
	p := sandboxTestPolicy(t, ws, home)

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

	root := cmd.Process.Pid
	pids := waitForPIDs(t, &stdout, 2)
	all := append([]int{root}, pids...)
	for _, pid := range all {
		if !processAlive(pid) {
			t.Fatalf("pid %d should be alive before cancellation", pid)
		}
	}
	if err := c.KillTree(cmd); err != nil {
		t.Fatalf("kill tree: %v", err)
	}
	_ = cmd.Wait()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		alive := false
		for _, pid := range all {
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
	t.Fatalf("descendants survived cancellation: root=%d children=%v", root, pids)
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
