//go:build linux

package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/process"
)

const hostileHelperEnv = "GO_WANT_HOSTILE_HELPER"

// TestHostileHelperProcess is the untrusted-workload fixture. It is inert unless
// GO_WANT_HOSTILE_HELPER=1. It deliberately tries to escape lifecycle ownership
// by stripping the ownership token from a child and calling setsid.
func TestHostileHelperProcess(t *testing.T) {
	if os.Getenv(hostileHelperEnv) != "1" {
		t.Skip("not a hostile helper invocation")
	}
	mode := os.Getenv("HOSTILE_MODE")
	arg := os.Getenv("HOSTILE_ARG")
	switch mode {
	case "strip-setsid", "strip-setsid-exit":
		marker := randomMarker()
		child := exec.Command(os.Args[0], "-test.run=TestHostileHelperProcess")
		child.Env = append(stripOwnershipEnv(os.Environ()),
			hostileHelperEnv+"=1", "HOSTILE_MODE=sleep", "HOSTILE_CHILD="+marker)
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := child.Start(); err != nil {
			os.Exit(4)
		}
		_ = os.WriteFile(arg, []byte(marker+"\n"), 0o600)
		if mode == "strip-setsid-exit" {
			// Stay alive briefly so a host-side test can observe the descendant,
			// then exit: the supervisor exits and the namespace is torn down.
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	case "sleep":
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	case "orphan-parent":
		runOrphanParent()
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func randomMarker() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// stripOwnershipEnv removes every Wayshard ownership marker so a descendant can
// evade any environment-token scan.
func stripOwnershipEnv(env []string) []string {
	var out []string
	for _, kv := range env {
		if strings.HasPrefix(kv, process.TokenEnv+"=") || strings.HasPrefix(kv, process.ToolTokenEnv+"=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// runOrphanParent launches a sandboxed hostile tree and then exits without
// reaping or killing it, simulating a server crash. The PID-namespace supervisor
// must detect the dead parent and tear the tree down.
func runOrphanParent() {
	ws := os.Getenv("HOSTILE_WS")
	home := os.Getenv("HOSTILE_HOME")
	pidfile := os.Getenv("HOSTILE_PIDFILE")
	exe, err := os.Executable()
	if err != nil {
		os.Exit(5)
	}
	p := ToolPolicy(ws, home, NetNone)
	p.ReadOnlyRoots = append(p.ReadOnlyRoots, filepath.Dir(exe))
	token, err := process.NewToken()
	if err != nil {
		os.Exit(5)
	}
	cmd := exec.Command(exe, "-test.run=TestHostileHelperProcess")
	cmd.Env = append(ToolEnv(home, home, map[string]string{process.TokenEnv: token}),
		hostileHelperEnv+"=1", "HOSTILE_MODE=strip-setsid", "HOSTILE_ARG="+pidfile)
	con := AsConstrainer(DefaultBackend())
	if _, err := con.Compile(p); err != nil {
		os.Exit(5)
	}
	if err := con.Constrain(cmd, p); err != nil {
		os.Exit(5)
	}
	if err := cmd.Start(); err != nil {
		os.Exit(6)
	}
	if _, err := con.Attach(cmd, p); err != nil {
		os.Exit(6)
	}
	marker := ""
	for i := 0; i < 500; i++ {
		if b, rerr := os.ReadFile(pidfile); rerr == nil {
			if m := strings.TrimSpace(string(b)); m != "" {
				marker = m
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Record the descendant's host pid (host-side scan) before crashing.
	if pidfile2 := os.Getenv("HOSTILE_PIDFILE2"); pidfile2 != "" && marker != "" {
		hostPid := 0
		for i := 0; i < 500; i++ {
			if hostPid = findHostPidByMarker(marker); hostPid > 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		_ = os.WriteFile(pidfile2, []byte(strconv.Itoa(hostPid)+"\n"), 0o600)
	}
	// Deliberately do not Wait/Kill: simulate a crashed server.
}

func hostileExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return exe
}

func hostilePolicy(t *testing.T, ws, home string) Policy {
	t.Helper()
	p := ToolPolicy(ws, home, NetNone)
	p.ReadOnlyRoots = append(p.ReadOnlyRoots, filepath.Dir(hostileExe(t)))
	return p
}

func waitMarker(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if m := strings.TrimSpace(string(b)); m != "" {
				return m
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("marker file %s was not written", path)
	return ""
}

// findHostPidByMarker returns the host PID of the (unique) process whose
// environment carries HOSTILE_CHILD=<marker>. The descendant lives in a PID
// namespace, so only a host-side scan can observe it.
func findHostPidByMarker(marker string) int {
	needle := "HOSTILE_CHILD=" + marker
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "environ"))
		if err != nil {
			continue
		}
		for _, kv := range strings.Split(string(b), "\x00") {
			if kv == needle {
				return pid
			}
		}
	}
	return 0
}

func waitHostPid(t *testing.T, marker string) int {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if pid := findHostPidByMarker(marker); pid > 0 {
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no host process carries marker %s", marker)
	return 0
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func waitPidDead(pid int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return !pidAlive(pid)
}

func environHasToken(pid int, token string) bool {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
	if err != nil {
		return false
	}
	for _, kv := range strings.Split(string(b), "\x00") {
		if kv == process.TokenEnv+"="+token || kv == process.ToolTokenEnv+"="+token {
			return true
		}
	}
	return false
}

// isNamespaceInit reports whether pid is PID 1 in its own PID namespace, read
// from the host /proc/<pid>/status NSpid (last field).
func isNamespaceInit(pid int) bool {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "NSpid:") {
			f := strings.Fields(line)
			if len(f) < 2 {
				return false
			}
			return f[len(f)-1] == "1"
		}
	}
	return false
}

// TestLinuxNamespaceBoundaryTerminatesTokenStrippedDescendant is the mandatory
// adversarial test: a required tool workload spawns a child with the ownership
// token removed and setsid, and the non-removable PID-namespace boundary must
// still terminate it on cancellation, with no token scan involved.
func TestLinuxNamespaceBoundaryTerminatesTokenStrippedDescendant(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skipf("required isolation unavailable: %s", c.Report().Detail)
	}
	token, err := process.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	home := t.TempDir()
	markerFile := filepath.Join(ws, "desc.marker")
	p := hostilePolicy(t, ws, home)

	cmd := exec.Command(hostileExe(t), "-test.run=TestHostileHelperProcess")
	cmd.Env = append(ToolEnv(home, home, map[string]string{process.TokenEnv: token}),
		hostileHelperEnv+"=1", "HOSTILE_MODE=strip-setsid", "HOSTILE_ARG="+markerFile)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
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

	supervisor := cmd.Process.Pid
	marker := waitMarker(t, markerFile)
	descendant := waitHostPid(t, marker)

	if !isNamespaceInit(supervisor) {
		t.Fatalf("supervisor %d is not PID-namespace init", supervisor)
	}
	if environHasToken(descendant, token) {
		t.Fatalf("fixture failed to strip the ownership token from descendant %d", descendant)
	}
	if !pidAlive(descendant) {
		t.Fatalf("descendant %d should be alive before cancellation", descendant)
	}

	if err := c.KillTree(cmd); err != nil {
		t.Fatalf("kill tree: %v", err)
	}
	_ = cmd.Wait()
	if !waitPidDead(descendant, 10*time.Second) {
		t.Fatalf("token-stripped setsid descendant %d survived cancellation", descendant)
	}
}

// TestLinuxNamespaceBoundaryTerminatesOnNormalCompletion proves the descendant
// dies when the target exits normally (the supervisor exits, tearing down the
// namespace), with no explicit cancellation.
func TestLinuxNamespaceBoundaryTerminatesOnNormalCompletion(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skipf("required isolation unavailable: %s", c.Report().Detail)
	}
	token, _ := process.NewToken()
	ws := t.TempDir()
	home := t.TempDir()
	markerFile := filepath.Join(ws, "desc.marker")
	p := hostilePolicy(t, ws, home)

	cmd := exec.Command(hostileExe(t), "-test.run=TestHostileHelperProcess")
	cmd.Env = append(ToolEnv(home, home, map[string]string{process.TokenEnv: token}),
		hostileHelperEnv+"=1", "HOSTILE_MODE=strip-setsid-exit", "HOSTILE_ARG="+markerFile)
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
	marker := waitMarker(t, markerFile)
	descendant := waitHostPid(t, marker)
	_ = cmd.Wait()
	if !waitPidDead(descendant, 10*time.Second) {
		t.Fatalf("token-stripped setsid descendant %d survived normal completion", descendant)
	}
}

// TestLinuxNamespaceBoundaryOnServerDeath proves the supervisor tears the tree
// down when the process that launched it dies (recovery), so no orphan survives
// a server crash even though the descendant stripped its ownership token.
func TestLinuxNamespaceBoundaryOnServerDeath(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skipf("required isolation unavailable: %s", c.Report().Detail)
	}
	ws := t.TempDir()
	home := t.TempDir()
	markerFile := filepath.Join(ws, "desc.marker")
	hostPidFile := filepath.Join(ws, "desc.hostpid")
	orphan := exec.Command(hostileExe(t), "-test.run=TestHostileHelperProcess")
	orphan.Env = append(os.Environ(),
		hostileHelperEnv+"=1", "HOSTILE_MODE=orphan-parent",
		"HOSTILE_WS="+ws, "HOSTILE_HOME="+home,
		"HOSTILE_PIDFILE="+markerFile, "HOSTILE_PIDFILE2="+hostPidFile)
	if err := orphan.Run(); err != nil {
		t.Fatalf("orphan parent: %v", err)
	}
	descendant := waitFileInt(t, hostPidFile)
	if descendant <= 0 {
		t.Fatalf("orphan parent did not record a host pid")
	}
	if !waitPidDead(descendant, 15*time.Second) {
		t.Fatalf("token-stripped descendant %d survived server death", descendant)
	}
}

func waitFileInt(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				return n
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("int file %s was not written", path)
	return 0
}

// TestLinuxNamespaceBoundaryOnLoopbackPolicy proves the same non-removable
// boundary applies to the isolated-loopback ACP probe policy (private netns +
// PID-namespace supervisor): a token-stripped setsid descendant is still
// terminated on cancellation.
func TestLinuxNamespaceBoundaryOnLoopbackPolicy(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available || !LoopbackProbeAvailable() {
		t.Skip("required isolation or loopback isolation unavailable")
	}
	token, _ := process.NewToken()
	home := t.TempDir()
	markerFile := filepath.Join(home, "desc.marker")
	p := LoopbackProbePolicy(hostileExe(t), home, home)
	p.ReadOnlyRoots = append(p.ReadOnlyRoots, filepath.Dir(hostileExe(t)))

	cmd := exec.Command(hostileExe(t), "-test.run=TestHostileHelperProcess")
	cmd.Env = append(ToolEnv(home, home, map[string]string{process.TokenEnv: token}),
		hostileHelperEnv+"=1", "HOSTILE_MODE=strip-setsid", "HOSTILE_ARG="+markerFile)
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
	marker := waitMarker(t, markerFile)
	descendant := waitHostPid(t, marker)
	if environHasToken(descendant, token) {
		t.Fatalf("fixture failed to strip the token")
	}
	if err := c.KillTree(cmd); err != nil {
		t.Fatalf("kill tree: %v", err)
	}
	_ = cmd.Wait()
	if !waitPidDead(descendant, 10*time.Second) {
		t.Fatalf("loopback-policy token-stripped descendant %d survived cancellation", descendant)
	}
}

// TestLinuxReconcileTokenHashSparesUnrelated proves PID-reuse safety: a
// reconciliation for a token that no process carries must not kill an unrelated
// process.
func TestLinuxReconcileTokenHashSparesUnrelated(t *testing.T) {
	if !process.Supported() {
		t.Skip("ownership unsupported")
	}
	unrelated := exec.Command("/bin/sleep", "300")
	if err := unrelated.Start(); err != nil {
		t.Skipf("sleep unavailable: %v", err)
	}
	defer func() { _ = unrelated.Process.Kill() }()
	pid := unrelated.Process.Pid

	randomHash, _ := process.NewToken()
	_, remaining, supported, err := process.ReconcileTokenHash(process.HashToken(randomHash), 0, 2*time.Second)
	if err != nil || !supported {
		t.Fatalf("reconcile: supported=%v err=%v", supported, err)
	}
	if remaining != 0 {
		t.Fatalf("unexpected remaining=%d", remaining)
	}
	if !pidAlive(pid) {
		t.Fatalf("unrelated process %d was killed by token reconciliation", pid)
	}
}
