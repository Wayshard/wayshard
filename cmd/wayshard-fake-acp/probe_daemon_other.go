//go:build !unix

package main

// spawnDetached is a no-op on platforms without POSIX sessions.
func spawnDetached(string) {}
