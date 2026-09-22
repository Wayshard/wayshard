//go:build windows

package sandbox

import (
	"fmt"
	"os"
)

// runDetachedHelper is only meaningful on POSIX; the setsid tests are
// darwin-only. This stub keeps the shared helper dispatch compiling on Windows.
func runDetachedHelper(mode string) {
	fmt.Printf("unsupported helper mode %q on windows\n", mode)
	os.Exit(2)
}
