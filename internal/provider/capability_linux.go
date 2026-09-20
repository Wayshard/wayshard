//go:build linux

package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
)

// Detect runtime-probes whether the platform can enforce provider-only
// networking. It attempts to start a trivial command inside a fresh user and
// network namespace: if the kernel or the runtime policy forbids that, the
// capability is reported unavailable and provider routes fail closed.
func Detect() domain.ProviderNetworkCapability {
	if runtime.GOOS != "linux" {
		return domain.ProviderNetworkCapability{Platform: runtime.GOOS, Reason: "provider networking is Linux-only"}
	}
	attr, err := NewUserNetNSAttr()
	if err != nil {
		return domain.ProviderNetworkCapability{Platform: "linux", Reason: err.Error()}
	}
	probe := "/bin/true"
	if _, err := os.Stat(probe); err != nil {
		probe = "/usr/bin/true"
	}
	if _, err := os.Stat(probe); err != nil {
		return domain.ProviderNetworkCapability{Platform: "linux", Reason: "no probe executable available"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, probe)
	cmd.SysProcAttr = attr
	out, err := cmd.CombinedOutput()
	if err != nil {
		reason := "network namespace unavailable"
		if s := strings.TrimSpace(string(out)); s != "" {
			reason += ": " + s
		}
		return domain.ProviderNetworkCapability{Platform: "linux", Reason: fmt.Sprintf("%s (%v)", reason, err)}
	}
	return domain.ProviderNetworkCapability{
		Available: true,
		Mode:      "netns_connect_broker",
		Platform:  "linux",
		Transport: "https_connect",
	}
}
