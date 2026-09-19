//go:build linux

package sandbox

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func startListeners(t *testing.T) (tcpAddr, udpAddr, fsPath, absName string) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	tcpAddr = ln.Addr().String()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	udpAddr = pc.LocalAddr().String()
	dir := t.TempDir()
	fsPath = filepath.Join(dir, "host.sock")
	us, err := net.Listen("unix", fsPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { us.Close() })
	absName = fmt.Sprintf("@wayshard-audit-%d", os.Getpid())
	ua, err := net.Listen("unix", absName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ua.Close() })
	return
}

func writeNetProbe(t *testing.T, ws, tcpAddr, udpAddr, fsPath, absName string) string {
	t.Helper()
	_, tcpPort, _ := net.SplitHostPort(tcpAddr)
	_, udpPort, _ := net.SplitHostPort(udpAddr)
	script := fmt.Sprintf(`
import socket
def res(fn, label):
    try:
        fn(); print(label + "=OK")
    except OSError as e:
        print(label + "=ERRNO" + str(e.errno))
res(lambda: socket.socket(socket.AF_INET, socket.SOCK_STREAM).connect(("127.0.0.1", %s)), "TCP")
res(lambda: socket.socket(socket.AF_INET, socket.SOCK_DGRAM).sendto(b"x", ("127.0.0.1", %s)), "UDP")
res(lambda: socket.socket(socket.AF_UNIX, socket.SOCK_STREAM).connect(%q), "UNIXFS")
res(lambda: socket.socket(socket.AF_UNIX, socket.SOCK_STREAM).connect(%q), "UNIXABS")
try:
    open("/proc/self/environ","rb").read(); print("PROC=OK")
except OSError as e:
    print("PROC=ERRNO" + str(e.errno))
`, tcpPort, udpPort, fsPath, "\x00"+strings.TrimPrefix(absName, "@"))
	p := filepath.Join(ws, "netprobe.py")
	if err := os.WriteFile(p, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestNetworkNoneEnforced proves kernel-level denial of TCP, UDP, filesystem
// AF_UNIX and abstract AF_UNIX for the process, its child and grandchild.
func TestNetworkNoneEnforced(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only")
	}
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skip("native sandbox unavailable")
	}
	tcpAddr, udpAddr, fsPath, absName := startListeners(t)
	ws := t.TempDir()
	home := t.TempDir()
	probe := writeNetProbe(t, ws, tcpAddr, udpAddr, fsPath, absName)
	pol := ToolPolicy(ws, home, NetNone)

	run := func(args ...string) string {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := runConstrained(t, c, cmd, pol)
		if err != nil {
			t.Fatalf("constrained run failed: %v\n%s", err, out)
		}
		return out
	}
	direct := run("/usr/bin/python3", probe)
	for _, label := range []string{"TCP", "UDP", "UNIXFS", "UNIXABS", "PROC"} {
		if !strings.Contains(direct, label+"=ERRNO") {
			t.Fatalf("%s not denied under NetworkNone:\n%s", label, direct)
		}
	}
	if strings.Contains(direct, "=OK") {
		t.Fatalf("NetworkNone allowed an external channel:\n%s", direct)
	}
	child := run("/bin/sh", "-c", "/usr/bin/python3 "+probe)
	if !strings.Contains(child, "TCP=ERRNO") || !strings.Contains(child, "UDP=ERRNO") || !strings.Contains(child, "UNIXABS=ERRNO") {
		t.Fatalf("child escaped NetworkNone:\n%s", child)
	}
	grand := run("/bin/sh", "-c", "/bin/sh -c '/usr/bin/python3 "+probe+"'")
	if !strings.Contains(grand, "TCP=ERRNO") || !strings.Contains(grand, "UDP=ERRNO") {
		t.Fatalf("grandchild escaped NetworkNone:\n%s", grand)
	}
}

// TestNetworkUnrestrictedStillWorks proves the fix did not destroy networking.
func TestNetworkUnrestrictedStillWorks(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only")
	}
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skip("native sandbox unavailable")
	}
	tcpAddr, udpAddr, fsPath, absName := startListeners(t)
	ws := t.TempDir()
	home := t.TempDir()
	probe := writeNetProbe(t, ws, tcpAddr, udpAddr, fsPath, absName)
	pol := ToolPolicy(ws, home, NetUnrestricted)
	cmd := exec.Command("/usr/bin/python3", probe)
	out, err := runConstrained(t, c, cmd, pol)
	if err != nil {
		t.Fatalf("unrestricted run failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "TCP=OK") || !strings.Contains(out, "UDP=OK") {
		t.Fatalf("unrestricted network did not work:\n%s", out)
	}
}

// TestUnsupportedNetworkPoliciesFailClosed proves allowlist/brokered are not
// silently downgraded.
func TestUnsupportedNetworkPoliciesFailClosed(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	for _, mode := range []NetworkMode{NetAllowlist, NetBrokered} {
		if _, err := c.Compile(ToolPolicy(t.TempDir(), t.TempDir(), mode)); err == nil {
			t.Fatalf("policy %q did not fail closed", mode)
		}
	}
}
