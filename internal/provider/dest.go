// Package provider implements Wayshard's enforceable provider-only networking
// capability. A provider-capable harness runs inside an isolated Linux network
// namespace whose only reachable endpoint is a per-attempt Wayshard broker. The
// broker validates every destination and dials it on the host side. Proxy
// environment variables carry the transport, but the namespace, not the proxy
// variables, is the security boundary.
package provider

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/Wayshard/wayshard/internal/domain"
)

// Destination is one authorized provider endpoint (host plus port).
type Destination struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// Policy is the set of provider destinations a run's broker will dial.
type Policy struct {
	Allowed []Destination `json:"allowed"`
}

// NormalizeHost lowercases a host and strips a single trailing dot so that
// "API.Example.com." and "api.example.com" compare equal.
func NormalizeHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	for strings.HasSuffix(h, ".") {
		h = strings.TrimSuffix(h, ".")
	}
	return h
}

// Authorize checks that the requested host and port match a configured
// destination. Port policy is explicit, so approving a host does not approve
// every port on it.
func (p Policy) Authorize(rawHost string, port int) (Destination, error) {
	if port <= 0 || port > 65535 {
		return Destination{}, fmt.Errorf("destination port %d out of range", port)
	}
	host := NormalizeHost(rawHost)
	if host == "" {
		return Destination{}, fmt.Errorf("destination host is empty")
	}
	for _, d := range p.Allowed {
		if NormalizeHost(d.Host) == host && d.Port == port {
			return d, nil
		}
	}
	return Destination{}, fmt.Errorf("destination %s:%d is not authorized", host, port)
}

// Eligible reports whether the policy can authorize anything at all. A route
// that needs provider networking with no configured destination must be blocked
// rather than launched.
func (p Policy) Eligible() bool { return len(p.Allowed) > 0 }

// PolicyFromDomains converts the domain-level destination list into a policy.
func PolicyFromDomains(dests []domain.ProviderDestination) Policy {
	p := Policy{}
	for _, d := range dests {
		p.Allowed = append(p.Allowed, Destination{Host: d.Host, Port: d.Port})
	}
	return p
}

// ValidateDomains validates a destination policy expressed as domain values.
func ValidateDomains(dests []domain.ProviderDestination) error {
	return PolicyFromDomains(dests).CheckConfigured()
}

// CheckConfigured verifies that every configured destination is a valid,
// non-private address. An operator who misconfigures a loopback/private
// destination gets a blocked route rather than a silently widened boundary.
func (p Policy) CheckConfigured() error {
	if !p.Eligible() {
		return fmt.Errorf("no provider destinations configured")
	}
	for _, d := range p.Allowed {
		if d.Port <= 0 || d.Port > 65535 {
			return fmt.Errorf("provider destination %s has invalid port %d", d.Host, d.Port)
		}
		host := NormalizeHost(d.Host)
		if host == "" {
			return fmt.Errorf("provider destination has empty host")
		}
		if addr, err := netip.ParseAddr(host); err == nil {
			if IsDisallowedAddr(addr) {
				return fmt.Errorf("provider destination %s is a disallowed address", d.Host)
			}
		}
	}
	return nil
}
