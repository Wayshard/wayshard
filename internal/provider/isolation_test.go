package provider

import (
	"io"
	"log/slog"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
)

// TestBrokerCrossRunIsolation proves two runs' brokers are distinct endpoints
// with distinct destination policies and bearer capabilities, so one run cannot
// use another's provider transport.
func TestBrokerCrossRunIsolation(t *testing.T) {
	echo := startEcho(t)
	pub := netip.MustParseAddr("93.184.216.34")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	bA, err := StartBroker(
		filepath.Join(t.TempDir(), "s"), "bearer-A",
		Policy{Allowed: []Destination{{Host: "a.test", Port: 443}}},
		&fakeResolver{addrs: map[string][]netip.Addr{"a.test": {pub}}}, mapDialer{target: echo}, log,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer bA.Close()
	bB, err := StartBroker(
		filepath.Join(t.TempDir(), "s"), "bearer-B",
		Policy{Allowed: []Destination{{Host: "b.test", Port: 443}}},
		&fakeResolver{addrs: map[string][]netip.Addr{"b.test": {pub}}}, mapDialer{target: echo}, log,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer bB.Close()

	if bA.SocketPath() == bB.SocketPath() {
		t.Fatal("brokers share a socket path")
	}
	// A cannot reach B's destination, and B cannot reach A's.
	if got := brokerRoundTrip(t, bA, "bearer-A", "CONNECT b.test:443 HTTP/1.1\r\n\r\n"); !strings.Contains(got, "403") {
		t.Fatalf("broker A allowed run B's destination: %q", got)
	}
	if got := brokerRoundTrip(t, bB, "bearer-A", "CONNECT a.test:443 HTTP/1.1\r\n\r\n"); !strings.Contains(got, "401") {
		t.Fatalf("broker B accepted run A's bearer: %q", got)
	}
}
