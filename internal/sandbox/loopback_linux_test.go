//go:build linux

package sandbox

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func loopbackProbeScript(hostPort, hostPort6 int) string {
	const tmpl = `
import socket
def res(label, fn):
    try:
        fn(); print(label+"=OK")
    except OSError as e:
        print(label+"=ERR%d" % e.errno)
def loopback_works():
    s = socket.socket(); s.bind(("127.0.0.1", 0)); s.listen(1)
    port = s.getsockname()[1]
    c = socket.create_connection(("127.0.0.1", port), timeout=2); c.close(); s.close()
res("LOOPBACK", loopback_works)
res("HOST_LOOPBACK", lambda: socket.create_connection(("127.0.0.1", __HOSTPORT__), timeout=2).close())
res("HOST_V6", lambda: socket.create_connection(("::1", __HOSTPORT6__), timeout=2).close())
res("PUBLIC", lambda: socket.create_connection(("93.184.216.34", 443), timeout=2).close())
res("LAN", lambda: socket.create_connection(("10.0.0.1", 443), timeout=2).close())
res("UDP", lambda: socket.socket(socket.AF_INET, socket.SOCK_DGRAM).close())
res("UNIX", lambda: socket.socket(socket.AF_UNIX, socket.SOCK_STREAM).close())
res("NETLINK", lambda: socket.socket(socket.AF_NETLINK, socket.SOCK_RAW).close())
res("PACKET", lambda: socket.socket(socket.AF_PACKET, socket.SOCK_RAW).close())
try:
    for line in open("/proc/net/route").read().splitlines()[1:]:
        if line.split()[1] == "00000000":
            print("DEFAULT_ROUTE=yes")
            break
    else:
        print("DEFAULT_ROUTE=no")
except Exception:
    print("DEFAULT_ROUTE=unknown")
`
	s := strings.ReplaceAll(tmpl, "__HOSTPORT6__", fmt.Sprintf("%d", hostPort6))
	s = strings.ReplaceAll(s, "__HOSTPORT__", fmt.Sprintf("%d", hostPort))
	return s
}

func startHostListeners(t *testing.T) (int, int) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	port := ln.Addr().(*net.TCPAddr).Port
	ln6, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		// No IPv6 host listener; still test ::1 refusal.
		return port, port
	}
	t.Cleanup(func() { _ = ln6.Close() })
	return port, ln6.Addr().(*net.TCPAddr).Port
}

// TestLoopbackProbeIsolation is the malicious-fixture proof for the ACP
// discovery loopback capability: isolated loopback works, while host loopback,
// host IPv6, public, LAN, UDP and host IPC remain unreachable, including from
// children and grandchildren.
func TestLoopbackProbeIsolation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only")
	}
	if !LoopbackProbeAvailable() {
		t.Skip("loopback isolation unavailable on this host")
	}
	hostPort, hostPort6 := startHostListeners(t)
	tmp := t.TempDir()
	script := filepath.Join(tmp, "probe.py")
	if err := os.WriteFile(script, []byte(loopbackProbeScript(hostPort, hostPort6)), 0o644); err != nil {
		t.Fatal(err)
	}
	pol := LoopbackProbePolicy("/usr/bin/python3", tmp, tmp)
	env := HarnessEnv(tmp, tmp, nil)

	run := func(args ...string) string {
		out, err := RunConstrainedOutput(context.Background(), pol, 20*time.Second, 128<<10, args[0], args[1:], env)
		if err != nil {
			t.Fatalf("constrained run %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	direct := run("/usr/bin/python3", script)
	t.Logf("loopback probe matrix:\n%s", direct)
	for _, want := range []string{"LOOPBACK=OK", "PUBLIC=ERR", "LAN=ERR", "UDP=ERR", "UNIX=ERR", "NETLINK=ERR", "PACKET=ERR"} {
		if !strings.Contains(direct, want) {
			t.Fatalf("missing %q in matrix:\n%s", want, direct)
		}
	}
	if strings.Contains(direct, "DEFAULT_ROUTE=yes") {
		t.Fatalf("loopback probe namespace had a default route:\n%s", direct)
	}
	for _, deny := range []string{"HOST_LOOPBACK=OK", "HOST_V6=OK", "PUBLIC=OK", "LAN=OK"} {
		if strings.Contains(direct, deny) {
			t.Fatalf("loopback probe reached %q:\n%s", deny, direct)
		}
	}
	// Child and grandchild inherit the same isolation.
	child := run("/bin/sh", "-c", "/usr/bin/python3 "+script)
	if !strings.Contains(child, "LOOPBACK=OK") || strings.Contains(child, "HOST_LOOPBACK=OK") || strings.Contains(child, "PUBLIC=OK") {
		t.Fatalf("child escaped loopback isolation:\n%s", child)
	}
	grand := run("/bin/sh", "-c", "/bin/sh -c '/usr/bin/python3 "+script+"'")
	if strings.Contains(grand, "HOST_LOOPBACK=OK") || strings.Contains(grand, "PUBLIC=OK") || strings.Contains(grand, "UDP=OK") {
		t.Fatalf("grandchild escaped loopback isolation:\n%s", grand)
	}
	// A version probe under ordinary ProbePolicy must remain NetworkNone.
	nonePol := ProbePolicy("/usr/bin/python3", tmp, tmp)
	noneOut, _ := RunConstrainedOutput(context.Background(), nonePol, 20*time.Second, 128<<10, "/usr/bin/python3", []string{script}, env)
	if strings.Contains(string(noneOut), "LOOPBACK=OK") {
		t.Fatalf("NetworkNone probe allowed a socket:\n%s", noneOut)
	}
	_ = exec.Command
}
