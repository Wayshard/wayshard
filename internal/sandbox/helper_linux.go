//go:build linux

package sandbox

import (
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func runHelper(args []string) bool {
	if len(args) < 5 || args[1] != HelperArg || args[3] != "--" {
		return false
	}
	policyFile := args[2]
	cmdPath := args[4]
	cmdArgs := args[4:]

	b, err := os.ReadFile(policyFile)
	_ = os.Remove(policyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-sandbox: read policy:", err)
		os.Exit(126)
	}
	var p Policy
	if err := json.Unmarshal(b, &p); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-sandbox: parse policy:", err)
		os.Exit(126)
	}
	if p.SyntheticHome != "" {
		_ = os.MkdirAll(p.SyntheticHome, 0o700)
	}
	if p.SyntheticTemp != "" {
		_ = os.MkdirAll(p.SyntheticTemp, 0o700)
	}
	if p.ProcIsolation {
		if !p.ProcNamespaced {
			// The backend did not create namespaces, so /proc cannot be scoped;
			// never grant the host procfs.
			p.ReadOnlyRoots = dropRoot(p.ReadOnlyRoots, "/proc")
		} else if err := setupProcIsolation(); err != nil {
			// Best-effort: if a scoped procfs cannot be mounted, do not grant the
			// host /proc. Existing (no-/proc) security is preserved.
			p.ReadOnlyRoots = dropRoot(p.ReadOnlyRoots, "/proc")
		}
	}
	if p.Network == NetLoopback {
		if !p.LoopbackNamespaced {
			fmt.Fprintln(os.Stderr, "wayshard-sandbox: loopback isolation was not applied")
			os.Exit(125)
		}
		if err := BringUpLoopback(); err != nil {
			fmt.Fprintln(os.Stderr, "wayshard-sandbox: required loopback isolation failed:", err)
			os.Exit(125)
		}
	}
	if err := applyLandlock(p); err != nil {
		if p.Required {
			fmt.Fprintln(os.Stderr, "wayshard-sandbox: required confinement failed:", err)
			os.Exit(125)
		}
	}
	switch p.Network {
	case NetNone:
		if err := applyNetworkNone(); err != nil {
			if p.Required {
				fmt.Fprintln(os.Stderr, "wayshard-sandbox: required network confinement failed:", err)
				os.Exit(125)
			}
		}
	case NetProvider:
		// Provider mode: the process is already inside its isolated network
		// namespace. Seccomp permits TCP to the in-namespace broker and denies
		// UDP, AF_UNIX, AF_NETLINK, AF_PACKET and io_uring.
		if err := applyNetworkProvider(); err != nil {
			if p.Required {
				fmt.Fprintln(os.Stderr, "wayshard-sandbox: required provider network confinement failed:", err)
				os.Exit(125)
			}
		}
	case NetLoopback:
		// Loopback mode: a private network namespace with only `lo`. The same
		// TCP-only seccomp filter applies, so only isolated loopback is
		// reachable.
		if err := applyNetworkProvider(); err != nil {
			if p.Required {
				fmt.Fprintln(os.Stderr, "wayshard-sandbox: required loopback network confinement failed:", err)
				os.Exit(125)
			}
		}
	case NetUnrestricted:
		if !p.AllowUnsafeHostNetwork {
			if p.Required {
				fmt.Fprintln(os.Stderr, "wayshard-sandbox: unrestricted host network requires explicit unsafe opt-in")
				os.Exit(125)
			}
		}
	default:
		// provider/allowlist/brokered/empty are never silently treated as
		// unrestricted.
		if p.Required {
			fmt.Fprintf(os.Stderr, "wayshard-sandbox: unsupported network mode %q\n", p.Network)
			os.Exit(125)
		}
	}
	if err := unix.Exec(cmdPath, cmdArgs, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-sandbox: exec:", err)
		os.Exit(127)
	}
	return true
}

// dropRoot removes a single path from a Landlock root list.
func dropRoot(roots []string, drop string) []string {
	out := roots[:0]
	for _, r := range roots {
		if r != drop {
			out = append(out, r)
		}
	}
	return out
}
