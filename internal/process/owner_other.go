//go:build !linux && !darwin

package process

import "time"

// Supported reports whether ownership can be verified on this platform. Windows
// cannot observe a process environment after the fact, so reconciliation is not
// authoritative and callers must treat it as unsupported.
func Supported() bool { return false }

// ReconcileTokenHash is not authoritative off Linux/macOS. It reports
// unsupported without killing anything, so callers can fail closed rather than
// guess ownership.
func ReconcileTokenHash(tokenHash string, pgid int, wait time.Duration) (observed, remaining int, supported bool, err error) {
	return 0, 0, false, nil
}

// ReconcileEnvToken is not authoritative off Linux/macOS.
func ReconcileEnvToken(envName, tokenHash string, pgid int, wait time.Duration) (observed, remaining int, supported bool, err error) {
	return 0, 0, false, nil
}
