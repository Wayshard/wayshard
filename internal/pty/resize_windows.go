//go:build windows

package pty

// Resize is a no-op on platforms without POSIX PTY window sizing.
func (s *Session) Resize(cols, rows int) error { return nil }
