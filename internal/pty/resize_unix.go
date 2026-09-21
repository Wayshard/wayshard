//go:build !windows

package pty

import gopty "github.com/creack/pty"

// Resize updates the PTY window size. Server-owned PTY only; the client never
// owns the process.
func (s *Session) Resize(cols, rows int) error {
	if s == nil || s.File == nil || cols <= 0 || rows <= 0 {
		return nil
	}
	return gopty.Setsize(s.File, &gopty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
