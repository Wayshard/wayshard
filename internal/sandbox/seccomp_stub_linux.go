//go:build linux && !(amd64 || arm64)

package sandbox

import "fmt"

// applyNetworkNone fails closed on Linux architectures without a verified
// seccomp filter implementation.
func applyNetworkNone() error {
	return fmt.Errorf("seccomp network confinement is only implemented for amd64 and arm64")
}

// applyNetworkProvider fails closed on Linux architectures without a verified
// seccomp filter implementation.
func applyNetworkProvider() error {
	return fmt.Errorf("seccomp provider network confinement is only implemented for amd64 and arm64")
}
