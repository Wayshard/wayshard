//go:build !linux

package provider

import (
	"fmt"
	"runtime"
	"syscall"

	"github.com/Wayshard/wayshard/internal/domain"
)

// NewUserNetNSAttr is unavailable off Linux.
func NewUserNetNSAttr() (*syscall.SysProcAttr, error) {
	return nil, fmt.Errorf("provider network shim is Linux-only")
}

// ShimMain never runs off Linux.
func ShimMain(string) int { return 2 }

// Detect reports provider networking as unavailable off Linux.
func Detect() domain.ProviderNetworkCapability {
	return domain.ProviderNetworkCapability{Platform: runtime.GOOS, Reason: "provider networking is Linux-only"}
}
