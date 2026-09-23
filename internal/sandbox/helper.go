package sandbox

import "os"

// HelperArg is the hidden argv marker for the in-binary sandbox helper. The
// helper applies the compiled policy and then execs the target, so the target
// and all of its descendants inherit the confinement.
const HelperArg = "__wayshard-sandbox-exec"

// SupervisorArg is the hidden argv marker for the PID-namespace supervisor. The
// supervisor is PID 1 in the target's PID namespace and remains init: the target
// runs as its child. Killing the supervisor tears down the namespace and every
// descendant with it (including setsid/double-forked processes), which is the
// non-removable lifecycle boundary for Linux required execution.
const SupervisorArg = "__wayshard-sandbox-supervise"

// ProcProbeArg is the hidden argv marker for the scoped-procfs capability probe.
const ProcProbeArg = "__wayshard-proc-probe"

// LoopbackProbeArg is the hidden argv marker for the loopback-namespace
// capability probe.
const LoopbackProbeArg = "__wayshard-loopback-probe"

// init makes the helper dispatch available in every binary that links this
// package (server, CLI, and test binaries), so os.Executable() is always a
// valid helper target.
func init() {
	if MaybeRunHelper(os.Args) {
		os.Exit(0)
	}
	if len(os.Args) >= 2 && os.Args[1] == ProcProbeArg {
		os.Exit(procProbeExit())
	}
	if len(os.Args) >= 2 && os.Args[1] == LoopbackProbeArg {
		os.Exit(loopbackProbeExit())
	}
}

// MaybeRunHelper runs the sandbox helper when invoked as
// `<binary> __wayshard-sandbox-exec <policy.json> -- <cmd> <args...>`. It
// returns true when it handled the invocation and main must not continue.
func MaybeRunHelper(args []string) bool { return runHelper(args) }
