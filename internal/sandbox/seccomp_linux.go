//go:build linux && (amd64 || arm64)

package sandbox

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Seccomp network confinement.
//
// Landlock ABI 4 only mediates TCP bind/connect. It does not mediate UDP or
// AF_UNIX (filesystem or abstract) sockets, so Landlock alone cannot implement
// NetworkNone. We install a seccomp-BPF filter immediately before exec that
// denies the creation of any communication socket, which the kernel enforces
// for the process, its children and grandchildren after exec.

const bpfJSET = 0x45 // BPF_JMP | BPF_JSET | BPF_K

const x32SyscallBit = 0x40000000

func seccompAuditArch() (uint32, error) {
	switch runtime.GOARCH {
	case "amd64":
		return unix.AUDIT_ARCH_X86_64, nil
	case "arm64":
		return unix.AUDIT_ARCH_AARCH64, nil
	default:
		return 0, fmt.Errorf("seccomp network confinement unsupported on %s", runtime.GOARCH)
	}
}

func seccompRet(code uint32, data uint32) uint32 {
	return code | (data & unix.SECCOMP_RET_DATA)
}

// applyNetworkNone installs a seccomp filter that returns EPERM for:
//   - socket(2) of any domain (AF_INET, AF_INET6, AF_UNIX, AF_NETLINK, ...),
//   - io_uring_setup(2), which can otherwise create sockets without socket(2).
//
// socketpair(2) is intentionally left permitted: it creates an anonymous,
// unreachable descriptor pair used for intra-process coordination and cannot
// reach a host endpoint.
func applyNetworkNone() error {
	arch, err := seccompAuditArch()
	if err != nil {
		return err
	}
	deny := seccompRet(unix.SECCOMP_RET_ERRNO, uint32(unix.EPERM))
	kill := uint32(unix.SECCOMP_RET_KILL_PROCESS)
	allow := uint32(unix.SECCOMP_RET_ALLOW)

	filter := []unix.SockFilter{
		// 0: load seccomp_data.arch
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 4},
		// 1: if arch == expected -> 3, else kill
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 1, Jf: 0, K: arch},
		{Code: unix.BPF_RET | unix.BPF_K, K: kill},
		// 3: load syscall number
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
		// 4: reject x32 ABI syscalls (nr has bit 0x40000000 set)
		{Code: bpfJSET, Jt: 2, Jf: 0, K: x32SyscallBit},
		// 5: socket(2) -> deny
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 1, Jf: 0, K: uint32(unix.SYS_SOCKET)},
		// 6: io_uring_setup(2) -> deny, else allow
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 0, Jf: 1, K: uint32(unix.SYS_IO_URING_SETUP)},
		// 7: deny
		{Code: unix.BPF_RET | unix.BPF_K, K: deny},
		// 8: allow
		{Code: unix.BPF_RET | unix.BPF_K, K: allow},
	}
	prog := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&prog)), 0, 0); err != nil {
		return fmt.Errorf("seccomp filter: %w", err)
	}
	return nil
}
