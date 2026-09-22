package testutil

import (
	"runtime"
	"testing"

	"github.com/Wayshard/wayshard/internal/sandbox"
)

// RequireNativeIsolation skips a test that needs to launch a harness, tool or
// probe under a required SandboxPolicy on a platform that cannot establish the
// required containment. On such platforms production execution fails closed
// (BLOCKED), so a test asserting a successful run is not meaningful there. The
// test still runs on platforms that enforce required isolation.
func RequireNativeIsolation(t testing.TB) {
	t.Helper()
	rep := sandbox.Probe()
	if !rep.Available {
		t.Skipf("native required isolation unavailable on %s: %s", runtime.GOOS, rep.Detail)
	}
}

// NativeIsolationAvailable reports whether required isolation can be enforced.
func NativeIsolationAvailable() bool { return sandbox.Probe().Available }
