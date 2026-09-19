package sandbox

import "os"

// HelperArg is the hidden argv marker for the in-binary sandbox helper. The
// helper applies the compiled policy and then execs the target, so the target
// and all of its descendants inherit the confinement.
const HelperArg = "__wayshard-sandbox-exec"

// init makes the helper dispatch available in every binary that links this
// package (server, CLI, and test binaries), so os.Executable() is always a
// valid helper target.
func init() {
	if MaybeRunHelper(os.Args) {
		os.Exit(0)
	}
}

// MaybeRunHelper runs the sandbox helper when invoked as
// `<binary> __wayshard-sandbox-exec <policy.json> -- <cmd> <args...>`. It
// returns true when it handled the invocation and main must not continue.
func MaybeRunHelper(args []string) bool { return runHelper(args) }
