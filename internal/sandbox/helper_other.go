//go:build !linux

package sandbox

func runHelper(args []string) bool { return false }

func helperPath() string { return "" }