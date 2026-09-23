//go:build linux

package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// gracefulShutdownGrace is how long the supervisor waits after forwarding
// SIGTERM to the target group before force-killing it on cancellation.
const gracefulShutdownGrace = 2 * time.Second

func runHelper(args []string) bool {
	if len(args) >= 2 && args[1] == SupervisorArg {
		return runSupervisor(args)
	}
	if len(args) < 5 || args[1] != HelperArg || args[3] != "--" {
		return false
	}
	policyFile := args[2]
	cmdPath := args[4]
	cmdArgs := args[4:]

	p, code := readPolicy(policyFile)
	if code != 0 {
		os.Exit(code)
	}
	if code := applyPolicy(p); code != 0 {
		os.Exit(code)
	}
	if err := unix.Exec(cmdPath, cmdArgs, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-sandbox: exec:", err)
		os.Exit(127)
	}
	return true
}

// runSupervisor is PID 1 in the target's PID namespace and remains init. It
// applies the policy to itself (inherited by the target), forks the target, reaps
// the whole tree as PID 1, and tears the namespace down when the target exits,
// when the server dies, or on a termination signal. Because a process cannot
// leave its PID namespace and killing namespace init terminates every remaining
// member, this boundary is not removable by untrusted code (no environment
// marker or process-group membership is trusted).
func runSupervisor(args []string) bool {
	// argv: <exe> SupervisorArg <policyFile> -- <cmd> <args...>
	if len(args) < 5 || args[3] != "--" {
		fmt.Fprintln(os.Stderr, "wayshard-sandbox: bad supervisor args")
		os.Exit(126)
	}
	policyFile := args[2]
	cmdPath := args[4]
	cmdArgs := args[4:]

	p, code := readPolicy(policyFile)
	if code != 0 {
		os.Exit(code)
	}
	if code := applyPolicy(p); code != 0 {
		os.Exit(code)
	}

	// Parent-death watchdog: the supervisor is trusted code, so unlike the
	// untrusted target it reliably tears the namespace down when the server (or
	// provider shim) that created it dies. getppid() cannot be used because a
	// PID-namespace init sees 0 for a parent outside its namespace, so the parent
	// holds the write end of a pipe whose read end is inherited as fd 3: EOF means
	// the parent is gone.
	if death := os.NewFile(3, "wayshard-death"); death != nil {
		unix.CloseOnExec(3) // the target must not inherit the death pipe
		go func() {
			buf := make([]byte, 1)
			for {
				if _, err := death.Read(buf); err != nil {
					os.Exit(137)
				}
			}
		}()
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM, syscall.SIGINT)

	child := exec.Command(cmdPath, cmdArgs[1:]...)
	child.Env = os.Environ()
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-sandbox: start target:", err)
		os.Exit(127)
	}
	childPid := child.Process.Pid

	exited := make(chan int, 1)
	go func() {
		// As PID 1 the supervisor must reap every child, including orphans
		// reparented to it by a double-fork. The target's status is reported.
		for {
			var ws syscall.WaitStatus
			pid, err := syscall.Wait4(-1, &ws, 0, nil)
			if err != nil {
				if err == syscall.EINTR {
					continue
				}
				return
			}
			if pid != childPid {
				continue
			}
			code := 0
			switch {
			case ws.Exited():
				code = ws.ExitStatus()
			case ws.Signaled():
				code = 128 + int(ws.Signal())
			}
			select {
			case exited <- code:
			default:
			}
		}
	}()

	select {
	case code := <-exited:
		os.Exit(code)
	case <-sigc:
		// Graceful shutdown: forward SIGTERM to the target group and give it a
		// bounded grace period to exit cleanly before force-killing. Either way
		// the supervisor then exits, which tears the namespace down.
		_ = syscall.Kill(-childPid, syscall.SIGTERM)
		_ = syscall.Kill(childPid, syscall.SIGTERM)
		select {
		case <-exited:
		case <-time.After(gracefulShutdownGrace):
			_ = syscall.Kill(-childPid, syscall.SIGKILL)
			_ = syscall.Kill(childPid, syscall.SIGKILL)
			select {
			case <-exited:
			case <-time.After(2 * time.Second):
			}
		}
		os.Exit(143)
	}
	return true
}

// readPolicy loads and removes the one-shot policy file.
func readPolicy(path string) (Policy, int) {
	var p Policy
	b, err := os.ReadFile(path)
	_ = os.Remove(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-sandbox: read policy:", err)
		return p, 126
	}
	if err := json.Unmarshal(b, &p); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-sandbox: parse policy:", err)
		return p, 126
	}
	return p, 0
}

// applyPolicy prepares namespace-scoped resources and applies Landlock/seccomp to
// the current process (inherited by the target and all descendants). It returns
// a non-zero exit code when a required policy cannot be applied.
func applyPolicy(p Policy) int {
	if p.SyntheticHome != "" {
		_ = os.MkdirAll(p.SyntheticHome, 0o700)
	}
	if p.SyntheticTemp != "" {
		_ = os.MkdirAll(p.SyntheticTemp, 0o700)
	}
	if p.ProcNamespaced {
		if p.ProcIsolation {
			if err := setupProcIsolation(); err != nil {
				// Best-effort: if a scoped procfs cannot be mounted, do not grant
				// the host /proc. Existing (no-/proc) security is preserved.
				p.ReadOnlyRoots = dropRoot(p.ReadOnlyRoots, "/proc")
			} else {
				// The scoped procfs shows only this PID namespace, so it is safe
				// to grant /proc for runtimes that require it.
				p.ReadOnlyRoots = appendRoot(p.ReadOnlyRoots, "/proc")
			}
		} else {
			// The namespace exists but the policy did not request /proc: deny it.
			p.ReadOnlyRoots = dropRoot(p.ReadOnlyRoots, "/proc")
		}
	} else if p.ProcIsolation {
		// No namespace: never grant the host procfs.
		p.ReadOnlyRoots = dropRoot(p.ReadOnlyRoots, "/proc")
	}
	if p.Network == NetLoopback {
		if !p.LoopbackNamespaced {
			fmt.Fprintln(os.Stderr, "wayshard-sandbox: loopback isolation was not applied")
			return 125
		}
		if err := BringUpLoopback(); err != nil {
			fmt.Fprintln(os.Stderr, "wayshard-sandbox: required loopback isolation failed:", err)
			return 125
		}
	}
	if err := applyLandlock(p); err != nil {
		if p.Required {
			fmt.Fprintln(os.Stderr, "wayshard-sandbox: required confinement failed:", err)
			return 125
		}
	}
	switch p.Network {
	case NetNone:
		if err := applyNetworkNone(); err != nil {
			if p.Required {
				fmt.Fprintln(os.Stderr, "wayshard-sandbox: required network confinement failed:", err)
				return 125
			}
		}
	case NetProvider, NetLoopback:
		// Provider/loopback mode: the process is already inside its isolated
		// network namespace. Seccomp permits TCP to the in-namespace broker and
		// denies UDP, AF_UNIX, AF_NETLINK, AF_PACKET and io_uring.
		if err := applyNetworkProvider(); err != nil {
			if p.Required {
				fmt.Fprintln(os.Stderr, "wayshard-sandbox: required network confinement failed:", err)
				return 125
			}
		}
	case NetUnrestricted:
		if !p.AllowUnsafeHostNetwork {
			if p.Required {
				fmt.Fprintln(os.Stderr, "wayshard-sandbox: unrestricted host network requires explicit unsafe opt-in")
				return 125
			}
		}
	default:
		// provider/allowlist/brokered/empty are never silently treated as
		// unrestricted.
		if p.Required {
			fmt.Fprintf(os.Stderr, "wayshard-sandbox: unsupported network mode %q\n", p.Network)
			return 125
		}
	}
	return 0
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

// appendRoot adds a path to a Landlock root list once.
func appendRoot(roots []string, add string) []string {
	for _, r := range roots {
		if r == add {
			return roots
		}
	}
	return append(roots, add)
}
