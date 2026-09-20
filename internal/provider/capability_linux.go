//go:build linux

package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/sandbox"
)

var (
	detectOnce   sync.Once
	detectResult domain.ProviderNetworkCapability
)

// Detect runtime-probes whether the platform can actually enforce provider-only
// networking by running the real shim setup in a fresh user+network namespace:
// raise loopback, bind the loopback proxy, and launch a trivial command under
// the provider policy. If the kernel or the runtime policy (for example an
// AppArmor unprivileged-userns restriction) forbids any part of that, the
// capability is reported unavailable and provider routes fail closed. The result
// is cached.
func Detect() domain.ProviderNetworkCapability {
	detectOnce.Do(func() { detectResult = detectOnceRun() })
	return detectResult
}

func detectOnceRun() domain.ProviderNetworkCapability {
	if runtime.GOOS != "linux" {
		return domain.ProviderNetworkCapability{Platform: runtime.GOOS, Reason: "provider networking is Linux-only"}
	}
	attr, err := NewUserNetNSAttr()
	if err != nil {
		return domain.ProviderNetworkCapability{Platform: "linux", Reason: err.Error()}
	}
	self, err := os.Executable()
	if err != nil {
		return domain.ProviderNetworkCapability{Platform: "linux", Reason: "cannot resolve server executable"}
	}
	trueBin := "/bin/true"
	if _, err := os.Stat(trueBin); err != nil {
		trueBin = "/usr/bin/true"
	}
	if _, err := os.Stat(trueBin); err != nil {
		return domain.ProviderNetworkCapability{Platform: "linux", Reason: "no probe executable available"}
	}
	dir, err := os.MkdirTemp("", "ws-prov-cap-")
	if err != nil {
		return domain.ProviderNetworkCapability{Platform: "linux", Reason: "probe temp dir unavailable"}
	}
	defer os.RemoveAll(dir)

	pol := sandbox.HarnessPolicy(dir, dir)
	pol.Network = sandbox.NetProvider
	port, err := RandomPort()
	if err != nil {
		return domain.ProviderNetworkCapability{Platform: "linux", Reason: err.Error()}
	}
	cfg := ShimConfig{
		BrokerSocket:   filepath.Join(dir, "s"),
		Bearer:         "capability-probe",
		ProxyPort:      port,
		Policy:         pol,
		HarnessCommand: trueBin,
		HarnessDir:     dir,
	}
	cfgPath, err := WriteShimConfig(dir, cfg)
	if err != nil {
		return domain.ProviderNetworkCapability{Platform: "linux", Reason: "probe config unavailable"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, ShimArg, cfgPath)
	cmd.SysProcAttr = attr
	out, err := cmd.CombinedOutput()
	if err != nil {
		reason := strings.TrimSpace(string(out))
		if reason == "" {
			reason = err.Error()
		}
		return domain.ProviderNetworkCapability{Platform: "linux", Reason: fmt.Sprintf("provider namespace setup unavailable: %s", reason)}
	}
	return domain.ProviderNetworkCapability{
		Available: true,
		Mode:      "netns_connect_broker",
		Platform:  "linux",
		Transport: "https_connect",
	}
}
