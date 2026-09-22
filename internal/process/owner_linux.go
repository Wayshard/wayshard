//go:build linux

package process

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"
)

// Supported reports whether ownership can be verified on this platform. On
// Linux the ownership token is observable through procfs, so reconciliation is
// authoritative.
func Supported() bool { return true }

// ReconcileTokenHash terminates every process carrying the attempt token whose
// digest matches tokenHash.
func ReconcileTokenHash(tokenHash string, pgid int, wait time.Duration) (observed, remaining int, supported bool, err error) {
	return ReconcileEnvToken(TokenEnv, tokenHash, pgid, wait)
}

// ReconcileEnvToken terminates every process carrying envName=token whose digest
// matches tokenHash, plus the recorded process group when it is proven owned. It
// waits until no owned process remains or the deadline elapses. Ownership is
// established from the process environment (the token is inherited by all
// descendants, including backgrounded or setsid descendants), and the digest is
// compared to the persisted hash so a reused PID or process-group id cannot
// cause an unrelated process to be killed. observed is the number of owned
// processes seen on the first scan.
func ReconcileEnvToken(envName, tokenHash string, pgid int, wait time.Duration) (observed, remaining int, supported bool, err error) {
	if tokenHash == "" {
		return 0, 0, true, nil
	}
	if wait <= 0 {
		wait = 3 * time.Second
	}
	deadline := time.Now().Add(wait)
	first := true
	for {
		pids := scanEnvTokenPids(envName, tokenHash)
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
			left := scanEnvTokenPids(envName, tokenHash)
			return observed, len(left), true, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// scanEnvTokenPids returns the PIDs of processes whose initial environment
// carries envName with a value whose digest matches tokenHash.
func scanEnvTokenPids(envName, tokenHash string) []int {
	prefix := []byte(envName + "=")
	var out []int
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
		if err != nil {
			continue
		}
		for _, kv := range bytes.Split(data, []byte{0}) {
			if !bytes.HasPrefix(kv, prefix) {
				continue
			}
			if HashToken(string(kv[len(prefix):])) == tokenHash {
				out = append(out, pid)
				break
			}
		}
	}
	return out
}

// groupHasEnvToken reports whether any process in the process group carries a
// token matching tokenHash, proving the group belongs to this record.
func groupHasEnvToken(envName string, pgid int, tokenHash string) bool {
	for _, pid := range scanEnvTokenPids(envName, tokenHash) {
		if processGroup(pid) == pgid {
			return true
		}
	}
	return false
}

// processGroup reads the process group id of pid from procfs.
func processGroup(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return -1
	}
	// Format: pid (comm) state ppid pgrp session ...; comm may contain spaces
	// and parentheses, so parse after the final ')'.
	i := bytes.LastIndexByte(data, ')')
	if i < 0 || i+2 >= len(data) {
		return -1
	}
	fields := bytes.Fields(data[i+2:])
	if len(fields) < 3 {
		return -1
	}
	n, err := strconv.Atoi(string(fields[2]))
	if err != nil {
		return -1
	}
	return n
}
