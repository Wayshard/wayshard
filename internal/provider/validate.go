package provider

import (
	"context"
	"fmt"
	"net"
	"net/netip"
)

// Resolver resolves a provider hostname on the trusted (host) side.
type Resolver interface {
	LookupNetIP(ctx context.Context, host string) ([]netip.Addr, error)
}

// Dialer opens the validated outbound connection. It is given a literal IP
// address (never a hostname) so the address that was validated is exactly the
// address that is dialed, closing DNS-rebinding races.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// DefaultResolver resolves using the host resolver.
type DefaultResolver struct{}

func (DefaultResolver) LookupNetIP(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// DefaultDialer dials with the standard net dialer.
type DefaultDialer struct{}

func (DefaultDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

var (
	denyV4 = mustPrefixes(
		"100.64.0.0/10", // shared address space (CGNAT)
		"192.0.0.0/24",  // IETF protocol assignments
		"192.0.2.0/24",  // TEST-NET-1 (documentation; not routable)
		"198.18.0.0/15", // benchmarking
		"198.51.100.0/24",
		"203.0.113.0/24",
		"240.0.0.0/4", // reserved
	)
	denyV6 = mustPrefixes(
		"2001::/32", // Teredo
		"2002::/16", // 6to4
	)
)

func mustPrefixes(ss ...string) []netip.Prefix {
	var out []netip.Prefix
	for _, s := range ss {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

// IsDisallowedAddr reports whether an address must never be reached through the
// provider broker: loopback, private, link-local, multicast, unspecified,
// broadcast, IPv4-mapped equivalents and IPv6 transition ranges that could
// embed a private IPv4 address.
func IsDisallowedAddr(a netip.Addr) bool {
	if !a.IsValid() {
		return true
	}
	a = a.Unmap()
	if a.Is4() {
		v4 := a.As4()
		if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
			a.IsMulticast() || a.IsUnspecified() {
			return true
		}
		if v4 == [4]byte{255, 255, 255, 255} {
			return true
		}
		for _, p := range denyV4 {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsMulticast() || a.IsUnspecified() || a.IsInterfaceLocalMulticast() {
		return true
	}
	for _, p := range denyV6 {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// ResolveValidated resolves host and returns the first address that passes the
// denylist, rejecting the whole destination if any resolved address is
// disallowed. It is called for every connection so a hostname that later
// resolves to a private address (DNS rebinding) is refused instead of cached.
func ResolveValidated(ctx context.Context, r Resolver, host string) (netip.Addr, error) {
	if r == nil {
		r = DefaultResolver{}
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if IsDisallowedAddr(addr) {
			return netip.Addr{}, fmt.Errorf("destination %s is a disallowed address", host)
		}
		return addr.Unmap(), nil
	}
	addrs, err := r.LookupNetIP(ctx, host)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return netip.Addr{}, fmt.Errorf("resolve %s: no addresses", host)
	}
	for _, a := range addrs {
		if IsDisallowedAddr(a) {
			return netip.Addr{}, fmt.Errorf("destination %s resolves to disallowed address %s", host, a)
		}
	}
	return addrs[0].Unmap(), nil
}
