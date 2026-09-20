package provider

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mapDialer struct{ target string }

func (d mapDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var nd net.Dialer
	return nd.DialContext(ctx, "tcp", d.target)
}

func startEcho(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); _ = c.Close() }()
		}
	}()
	return ln.Addr().String()
}

// shortDir returns a short temporary directory. Filesystem Unix socket paths are
// limited (~104 bytes on macOS, ~108 on Linux), and t.TempDir embeds the full
// test name, which can exceed that.
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "wsb-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

func startTestBroker(t *testing.T, resolver Resolver, dialer Dialer) *Broker {
	t.Helper()
	dir := shortDir(t)
	b, err := StartBroker(
		filepath.Join(dir, "s"), "test-bearer",
		Policy{Allowed: []Destination{{Host: "provider.test", Port: 443}}},
		resolver, dialer, slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func brokerRoundTrip(t *testing.T, b *Broker, bearer, request string) string {
	t.Helper()
	c, err := net.Dial("unix", b.SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, bearer+"\n"+request); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(line)
}

func TestBrokerRejectsBadBearerAndMethods(t *testing.T) {
	echo := startEcho(t)
	res := &fakeResolver{addrs: map[string][]netip.Addr{"provider.test": {netip.MustParseAddr("93.184.216.34")}}}
	b := startTestBroker(t, res, mapDialer{target: echo})

	if got := brokerRoundTrip(t, b, "wrong", "CONNECT provider.test:443 HTTP/1.1\r\n\r\n"); !strings.Contains(got, "401") {
		t.Fatalf("bad bearer accepted: %q", got)
	}
	if got := brokerRoundTrip(t, b, "test-bearer", "GET http://provider.test/ HTTP/1.1\r\nHost: provider.test\r\n\r\n"); !strings.Contains(got, "405") {
		t.Fatalf("non-CONNECT accepted: %q", got)
	}
	if got := brokerRoundTrip(t, b, "test-bearer", "CONNECT evil.test:443 HTTP/1.1\r\n\r\n"); !strings.Contains(got, "403") {
		t.Fatalf("unauthorized destination accepted: %q", got)
	}
	if got := brokerRoundTrip(t, b, "test-bearer", "CONNECT provider.test:8443 HTTP/1.1\r\n\r\n"); !strings.Contains(got, "403") {
		t.Fatalf("unauthorized port accepted: %q", got)
	}
}

func TestBrokerRejectsDisallowedResolvedAddress(t *testing.T) {
	echo := startEcho(t)
	res := &fakeResolver{addrs: map[string][]netip.Addr{"provider.test": {netip.MustParseAddr("127.0.0.1")}}}
	b := startTestBroker(t, res, mapDialer{target: echo})
	if got := brokerRoundTrip(t, b, "test-bearer", "CONNECT provider.test:443 HTTP/1.1\r\n\r\n"); !strings.Contains(got, "403") {
		t.Fatalf("rebound private address accepted: %q", got)
	}
}

func TestBrokerTunnelsAuthorizedConnect(t *testing.T) {
	echo := startEcho(t)
	res := &fakeResolver{addrs: map[string][]netip.Addr{"provider.test": {netip.MustParseAddr("93.184.216.34")}}}
	b := startTestBroker(t, res, mapDialer{target: echo})

	c, err := net.Dial("unix", b.SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, "test-bearer\nCONNECT provider.test:443 HTTP/1.1\r\nHost: provider.test:443\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil || !strings.Contains(line, "200") {
		t.Fatalf("CONNECT not established: %q %v", line, err)
	}
	// Consume the rest of the CONNECT response headers.
	for {
		h, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if h == "\r\n" || h == "\n" {
			break
		}
	}
	if _, err := io.WriteString(c, "ping"); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(br, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "ping" {
		t.Fatalf("tunnel corrupted: %q", string(buf))
	}
	_ = fmt.Sprint()
}

// brokerStatusRaw writes a raw request (tolerating a write error when the broker
// closes early) and returns the first response line.
func brokerStatusRaw(t *testing.T, b *Broker, bearer, req string) string {
	t.Helper()
	c, err := net.Dial("unix", b.SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(c, bearer+"\n"+req)
	line, _ := bufio.NewReader(c).ReadString('\n')
	return strings.TrimSpace(line)
}

// TestBrokerBoundsConnectHeaders proves the CONNECT/bearer header phase is
// bounded: normal requests succeed while oversized single or aggregate headers
// are rejected without harming the broker.
func TestBrokerBoundsConnectHeaders(t *testing.T) {
	pub := netip.MustParseAddr("93.184.216.34")
	res := &fakeResolver{addrs: map[string][]netip.Addr{"provider.test": {pub}}}
	b := startTestBroker(t, res, mapDialer{target: startEcho(t)})

	if got := brokerRoundTrip(t, b, "test-bearer", "CONNECT provider.test:443 HTTP/1.1\r\nHost: provider.test:443\r\nX-Normal: 1\r\n\r\n"); !strings.Contains(got, "200") {
		t.Fatalf("normal CONNECT with headers rejected: %q", got)
	}
	if got := brokerStatusRaw(t, b, "test-bearer", "CONNECT provider.test:443 HTTP/1.1\r\nX-Big: "+strings.Repeat("A", 128<<10)+"\r\n\r\n"); !strings.Contains(got, "431") {
		t.Fatalf("oversized single header not rejected: %q", got)
	}
	var agg strings.Builder
	agg.WriteString("CONNECT provider.test:443 HTTP/1.1\r\n")
	for i := 0; i < 4000; i++ {
		fmt.Fprintf(&agg, "X-%d: %s\r\n", i, strings.Repeat("b", 64))
	}
	agg.WriteString("\r\n")
	if got := brokerStatusRaw(t, b, "test-bearer", agg.String()); !strings.Contains(got, "431") {
		t.Fatalf("excessive aggregate headers not rejected: %q", got)
	}
	// Broker remains healthy.
	if got := brokerRoundTrip(t, b, "test-bearer", "CONNECT provider.test:443 HTTP/1.1\r\n\r\n"); !strings.Contains(got, "200") {
		t.Fatalf("broker unhealthy after oversized headers: %q", got)
	}
}
