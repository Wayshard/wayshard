//go:build !linux

package sandbox

func runHelper(args []string) bool { return false }

func helperPath() string { return "" }

// procProbeExit is unsupported off Linux.
func procProbeExit() int { return 1 }

// loopbackProbeExit is unsupported off Linux.
func loopbackProbeExit() int { return 1 }

// LoopbackProbeAvailable is false off Linux.
func LoopbackProbeAvailable() bool { return false }
