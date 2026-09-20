package provider

import (
	"context"
	"fmt"
	"net/netip"
	"testing"
)

func TestIsDisallowedAddr(t *testing.T) {
	disallowed := []string{
		"127.0.0.1", "127.1.2.3", "0.0.0.0", "255.255.255.255",
		"10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.1.1",
		"100.64.0.1", "192.0.0.1", "198.18.0.1", "240.0.0.1",
		"::1", "::", "fe80::1", "fc00::1", "fd00::1", "ff02::1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:192.168.1.1",
		"2002:7f00:1::1", // 6to4 embedding 127.0.0.1
		"2001::1",        // Teredo
	}
	for _, s := range disallowed {
		a, err := netip.ParseAddr(s)
		if err != nil {
			t.Fatalf("parse %s: %v", s, err)
		}
		if !IsDisallowedAddr(a) {
			t.Errorf("expected %s to be disallowed", s)
		}
	}
	allowed := []string{"93.184.216.34", "1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"}
	for _, s := range allowed {
		a, _ := netip.ParseAddr(s)
		if IsDisallowedAddr(a) {
			t.Errorf("expected %s to be allowed", s)
		}
	}
}

type fakeResolver struct {
	addrs map[string][]netip.Addr
	calls int
}

func (f *fakeResolver) LookupNetIP(_ context.Context, host string) ([]netip.Addr, error) {
	f.calls++
	if a, ok := f.addrs[host]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("no such host")
}

func TestResolveValidatedRejectsRebindingAndPrivate(t *testing.T) {
	ctx := context.Background()
	pub := netip.MustParseAddr("93.184.216.34")
	lo := netip.MustParseAddr("127.0.0.1")

	// Literal private IP is rejected without any resolver call.
	r := &fakeResolver{}
	if _, err := ResolveValidated(ctx, r, "127.0.0.1"); err == nil {
		t.Fatal("literal loopback accepted")
	}
	if _, err := ResolveValidated(ctx, r, "::1"); err == nil {
		t.Fatal("literal IPv6 loopback accepted")
	}
	if r.calls != 0 {
		t.Fatalf("resolver consulted for literal IP: %d calls", r.calls)
	}

	// Initial public resolution succeeds.
	r.addrs = map[string][]netip.Addr{"provider.test": {pub}}
	got, err := ResolveValidated(ctx, r, "provider.test")
	if err != nil || got != pub {
		t.Fatalf("public resolution failed: %v %v", got, err)
	}

	// DNS rebinding: the same name now resolves loopback; every new connection
	// must revalidate and refuse.
	r.addrs["provider.test"] = []netip.Addr{lo}
	if _, err := ResolveValidated(ctx, r, "provider.test"); err == nil {
		t.Fatal("rebound loopback accepted")
	}

	// A hostname resolving to a mix of public and private is refused entirely.
	r.addrs["provider.test"] = []netip.Addr{pub, lo}
	if _, err := ResolveValidated(ctx, r, "provider.test"); err == nil {
		t.Fatal("mixed public/private resolution accepted")
	}
}
