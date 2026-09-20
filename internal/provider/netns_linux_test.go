//go:build linux

package provider

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/sandbox"
	"golang.org/x/sys/unix"
)

// TestMain lets the provider test binary act as the in-namespace shim and as the
// harness fixture when spawned by the enforcement test.
func TestMain(m *testing.M) {
	if os.Getenv("GO_PROVIDER_HARNESS") == "1" {
		os.Exit(providerHarnessHelper())
	}
	if os.Getenv("GO_PROVIDER_SHIM") == "1" {
		os.Exit(ShimMain(os.Getenv("GO_PROVIDER_SHIM_CFG")))
	}
	os.Exit(m.Run())
}

// providerHarnessHelper is a deliberately malicious provider harness fixture: it
// ignores proxy configuration and attempts direct network/IPC access before
// using the sanctioned transport.
func providerHarnessHelper() int {
	role := os.Getenv("PROBE_ROLE")
	public := os.Getenv("PROBE_PUBLIC_ADDR")
	proxy := os.Getenv("PROBE_PROXY_ADDR")

	if role == "child" || role == "grand" {
		label := "CHILD"
		if role == "grand" {
			label = "GRAND"
		}
		fmt.Printf("%s_DIRECT_PUBLIC=%s\n", label, deniedResult(tryTCP(public)))
		return 0
	}

	fmt.Printf("DIRECT_LOOPBACK=%s\n", deniedResult(tryTCP(os.Getenv("PROBE_DIRECT_LOOPBACK"))))
	fmt.Printf("DIRECT_PUBLIC=%s\n", deniedResult(tryTCP(public)))
	fmt.Printf("UDP=%s\n", deniedResult(trySocketErrno(unix.AF_INET, unix.SOCK_DGRAM)))
	fmt.Printf("UNIX=%s\n", deniedResult(trySocketErrno(unix.AF_UNIX, unix.SOCK_STREAM)))
	fmt.Printf("NETLINK=%s\n", deniedResult(trySocketErrno(unix.AF_NETLINK, unix.SOCK_RAW)))
	fmt.Printf("PACKET=%s\n", deniedResult(trySocketErrno(unix.AF_PACKET, unix.SOCK_RAW)))
	fmt.Printf("DNS=%s\n", deniedResult(tryDNS("provider.test")))
	fmt.Printf("PROXY=%s\n", proxyResult(proxy))

	spawnChild("child", public, false)
	spawnChild("grand", public, true)
	return 0
}

func deniedResult(err error) string {
	if err == nil {
		return "allowed"
	}
	return "denied"
}

func tryTCP(addr string) error {
	if addr == "" {
		return fmt.Errorf("no address")
	}
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	_ = c.Close()
	return nil
}

func trySocketErrno(domain, typ int) error {
	fd, err := unix.Socket(domain, typ, 0)
	if err != nil {
		return err
	}
	_ = unix.Close(fd)
	return nil
}

// tryDNS attempts direct name resolution. The untrusted harness has no
// broker-mediated DNS, so any resolution attempt must fail.
func tryDNS(host string) error {
	_, err := net.LookupHost(host)
	return err
}

func proxyResult(proxy string) string {
	if proxy == "" {
		return "missing"
	}
	c, err := net.DialTimeout("tcp", proxy, 2*time.Second)
	if err != nil {
		return "unreachable"
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.WriteString(c, "CONNECT provider.test:443 HTTP/1.1\r\nHost: provider.test:443\r\n\r\n"); err != nil {
		return "write_error"
	}
	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil || !strings.Contains(line, "200") {
		return "refused"
	}
	for {
		h, err := br.ReadString('\n')
		if err != nil {
			return "refused"
		}
		if h == "\r\n" || h == "\n" {
			break
		}
	}
	if _, err := io.WriteString(c, "ping"); err != nil {
		return "tunnel_write_error"
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(br, buf); err != nil {
		return "tunnel_read_error"
	}
	if string(buf) != "ping" {
		return "corrupt"
	}
	return "allowed"
}

func spawnChild(role, public string, setsid bool) {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "spawn %s: executable: %v\n", role, err)
		return
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "GO_PROVIDER_HARNESS=1", "PROBE_ROLE="+role, "PROBE_PUBLIC_ADDR="+public)
	if setsid {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "spawn %s: %v out=%q\n", role, err, string(out))
		return
	}
	fmt.Print(string(out))
}

// TestProviderNetnsEnforcement is the mandatory bypass proof: a malicious
// harness inside the provider namespace cannot reach the network directly,
// localhost, UDP, Unix sockets, netlink or packet sockets, but succeeds through
// the sanctioned broker transport to the approved simulated provider.
func TestProviderNetnsEnforcement(t *testing.T) {
	capability := Detect()
	if !capability.Available {
		t.Skipf("provider capability unavailable: %s", capability.Reason)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	echo := startEcho(t)
	_, echoPort, _ := net.SplitHostPort(echo)

	resolver := &fakeResolver{addrs: map[string][]netip.Addr{"provider.test": {netip.MustParseAddr("93.184.216.34")}}}
	work := t.TempDir()
	bearer, err := RandomBearer()
	if err != nil {
		t.Fatal(err)
	}
	broker, err := StartBroker(
		filepath.Join(work, "s"), bearer,
		Policy{Allowed: []Destination{{Host: "provider.test", Port: 443}}},
		resolver, mapDialer{target: echo},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = broker.Close() })

	proxyPort, err := RandomPort()
	if err != nil {
		t.Fatal(err)
	}
	ws := filepath.Join(work, "ws")
	synthetic := filepath.Join(work, "tmp")
	_ = os.MkdirAll(ws, 0o700)
	_ = os.MkdirAll(synthetic, 0o700)

	pol := sandbox.HarnessPolicy(ws, synthetic)
	pol.Network = sandbox.NetProvider
	pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, filepath.Dir(exe))

	// Wrapper script: the shim launches it under the sandbox policy; it execs
	// the test binary as the harness fixture.
	wrapper := filepath.Join(ws, "harness.sh")
	script := "#!/bin/sh\nexport GO_PROVIDER_HARNESS=1\nexec " + exe + "\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := ShimConfig{
		BrokerSocket:   broker.SocketPath(),
		Bearer:         bearer,
		ProxyPort:      proxyPort,
		Policy:         pol,
		HarnessCommand: wrapper,
		HarnessDir:     ws,
	}
	cfgPath, err := WriteShimConfig(work, cfg)
	if err != nil {
		t.Fatal(err)
	}

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", proxyPort)
	env := append(os.Environ(),
		"GO_PROVIDER_SHIM=1",
		"GO_PROVIDER_SHIM_CFG="+cfgPath,
		"PROBE_PROXY_ADDR="+proxyAddr,
		"PROBE_DIRECT_LOOPBACK=127.0.0.1:"+echoPort,
		"PROBE_PUBLIC_ADDR=93.184.216.34:443",
		"PROBE_UNIX_PATH="+filepath.Join(work, "x.sock"),
		"HTTPS_PROXY=http://"+proxyAddr,
		"HTTP_PROXY=http://"+proxyAddr,
		"NO_PROXY=",
	)
	attr, err := NewUserNetNSAttr()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = env
	cmd.SysProcAttr = attr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("shim run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	out := stdout.String()
	t.Logf("provider bypass matrix:\n%s", out)
	for _, want := range []string{
		"DIRECT_LOOPBACK=denied",
		"DIRECT_PUBLIC=denied",
		"UDP=denied",
		"UNIX=denied",
		"NETLINK=denied",
		"PACKET=denied",
		"DNS=denied",
		"PROXY=allowed",
		"CHILD_DIRECT_PUBLIC=denied",
		"GRAND_DIRECT_PUBLIC=denied",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("bypass matrix missing %q\nstdout:\n%s\nstderr:\n%s", want, out, stderr.String())
		}
	}
}
