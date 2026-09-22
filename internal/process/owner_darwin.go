//go:build darwin

package process

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// macOS exposes the initial environment of same-user processes through
// sysctl(KERN_PROCARGS2). That lets Wayshard identify an owned process tree by
// the per-attempt WAYSHARD_OWNER_TOKEN it inherited — including descendants
// that setsid/setpgid/double-fork away from the launch process group — instead
// of relying on PID or process group. Ownership is compared by token digest, so
// a reused PID cannot cause an unrelated process to be killed.
//
// The capability is probed once by reading our own environment. If a supported
// macOS build cannot expose same-user environments, Supported() returns false
// and required isolation fails closed rather than claiming process-tree
// ownership.

var ownershipProbed bool
var ownershipOK bool

// Supported reports whether token-based ownership is observable on this host.
func Supported() bool {
	if ownershipProbed {
		return ownershipOK
	}
	ownershipProbed = true
	env, err := procEnv(os.Getpid())
	if err != nil || len(env) == 0 {
		ownershipOK = false
		return false
	}
	// The enumeration API must also work; otherwise a scan failure could be
	// mistaken for "no owned processes".
	if _, err := unix.SysctlKinfoProcSlice("kern.proc.all"); err != nil {
		ownershipOK = false
		return false
	}
	ownershipOK = true
	return true
}

// ReconcileTokenHash terminates every process carrying the attempt token whose
// digest matches tokenHash.
func ReconcileTokenHash(tokenHash string, pgid int, wait time.Duration) (observed, remaining int, supported bool, err error) {
	return ReconcileEnvToken(TokenEnv, tokenHash, pgid, wait)
}

// ReconcileEnvToken terminates every process carrying envName=token whose digest
// matches tokenHash, plus the recorded process group when it is proven owned.
// observed is the number of owned processes seen on the first scan.
func ReconcileEnvToken(envName, tokenHash string, pgid int, wait time.Duration) (observed, remaining int, supported bool, err error) {
	if tokenHash == "" {
		return 0, 0, true, nil
	}
	if !Supported() {
		return 0, 0, false, nil
	}
	if wait <= 0 {
		wait = 3 * time.Second
	}
	deadline := time.Now().Add(wait)
	first := true
	for {
		pids, ok := scanEnvTokenPids(envName, tokenHash)
		if !ok {
			// Enumeration failed: cannot prove the tree is gone. Fail closed.
			return 0, 0, false, nil
		}
		if first {
			observed = len(pids)
			first = false
		}
		if len(pids) == 0 {
			return observed, 0, true, nil
		}
		groupOwned := pgid > 0 && groupHasEnvToken(envName, pgid, tokenHash)
		for _, pid := range pids {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		if groupOwned {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
		if time.Now().After(deadline) {
			left, ok := scanEnvTokenPids(envName, tokenHash)
			if !ok {
				return 0, 0, false, nil
			}
			return observed, len(left), true, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// scanEnvTokenPids returns the PIDs of processes whose initial environment
// carries envName with a value whose digest matches tokenHash, and whether the
// enumeration succeeded.
func scanEnvTokenPids(envName, tokenHash string) ([]int, bool) {
	self := os.Getpid()
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, false
	}
	prefix := envName + "="
	var out []int
	for _, kp := range kps {
		pid := int(kp.Proc.P_pid)
		if pid <= 0 || pid == self {
			continue
		}
		env, err := procEnv(pid)
		if err != nil {
			continue
		}
		for _, kv := range env {
			if !strings.HasPrefix(kv, prefix) {
				continue
			}
			if HashToken(kv[len(prefix):]) == tokenHash {
				out = append(out, pid)
				break
			}
		}
	}
	return out, true
}

func groupHasEnvToken(envName string, pgid int, tokenHash string) bool {
	pids, ok := scanEnvTokenPids(envName, tokenHash)
	if !ok {
		return false
	}
	for _, pid := range pids {
		if g, err := unix.Getpgid(pid); err == nil && g == pgid {
			return true
		}
	}
	return false
}

// procEnv returns the initial environment strings of pid. It retries a bounded
// number of times because the kernel size query can race a growing environment.
func procEnv(pid int) ([]string, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		buf, err := unix.SysctlRaw("kern.procargs2", pid)
		if err == nil {
			return parseProcargs2(buf), nil
		}
		lastErr = err
		if err != unix.ENOMEM {
			return nil, err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil, lastErr
}

// parseProcargs2 parses the KERN_PROCARGS2 layout:
//
//	[int32 argc][exec_path\0][padding\0...][argv[0]\0..argv[argc-1]\0][env\0...]
func parseProcargs2(buf []byte) []string {
	if len(buf) < 4 {
		return nil
	}
	argc := int(int32(binary.LittleEndian.Uint32(buf[:4])))
	if argc < 0 || argc > 1<<20 {
		return nil
	}
	rest := buf[4:]
	// Skip the executable path (NUL-terminated) and any alignment padding.
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return nil
	}
	rest = rest[i+1:]
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	// Skip argc argv strings.
	for n := 0; n < argc && len(rest) > 0; n++ {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			return nil
		}
		rest = rest[j+1:]
	}
	// The remainder are NUL-terminated environment strings.
	var env []string
	for len(rest) > 0 {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			break
		}
		if j > 0 {
			env = append(env, string(rest[:j]))
		}
		rest = rest[j+1:]
	}
	return env
}
