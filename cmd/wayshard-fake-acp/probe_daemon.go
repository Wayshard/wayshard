package main

import (
	"os"
	"time"
)

// maybeSpawnProbeDaemon is a test-only fixture: when WAYSHARD_FAKE_PROBE_DAEMON
// names a marker, the fake spawns a session-detached (setsid) descendant whose
// command line contains the marker, then optionally blocks so a caller can
// SIGKILL the server while the descendant is still alive. The descendant
// inherits the probe ownership token, so startup reconciliation must find and
// terminate it by token after a crash.
func maybeSpawnProbeDaemon() {
	marker := os.Getenv("WAYSHARD_FAKE_PROBE_DAEMON")
	if marker == "" {
		return
	}
	spawnDetached(marker)
	if os.Getenv("WAYSHARD_FAKE_PROBE_DAEMON_HANG") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
}
