//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Landlock syscall numbers are identical across linux/amd64, arm64, 386 and arm.
const (
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446

	landlockCreateRulesetVersion = 1
	landlockRuleTypePathBeneath  = 1
)

type landlockRulesetAttr struct {
	HandledAccessFS  uint64
	HandledAccessNet uint64
}

type landlockPathBeneathAttr struct {
	AllowedAccess uint64
	ParentFd      int32
	Reserved      uint32
}

func landlockABI() (int, error) {
	r, _, errno := unix.Syscall(sysLandlockCreateRuleset, 0, 0, landlockCreateRulesetVersion)
	if errno != 0 {
		return 0, errno
	}
	return int(r), nil
}

func landlockAccessAll(abi int) uint64 {
	m := uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE |
		unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
		unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
		unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
		unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM)
	if abi >= 2 {
		m |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		m |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	return m
}

func landlockReadOnly() uint64 {
	return uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE |
		unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR)
}

// applyLandlock installs a ruleset that permits exactly the policy's roots and
// denies every other filesystem access for this process and all descendants.
func applyLandlock(p Policy) error {
	abi, err := landlockABI()
	if err != nil {
		return fmt.Errorf("landlock unavailable: %w", err)
	}
	handled := landlockAccessAll(abi)
	handledNet := uint64(0)
	if p.Network == NetNone {
		if abi < 4 {
			return fmt.Errorf("network denial requires landlock ABI>=4 (have %d)", abi)
		}
		handledNet = uint64(unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP)
	}
	attr := landlockRulesetAttr{HandledAccessFS: handled, HandledAccessNet: handledNet}
	fd, _, errno := unix.Syscall(sysLandlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	rulesetFd := int(fd)
	defer unix.Close(rulesetFd)

	add := func(path string, access uint64) error {
		if path == "" {
			return nil
		}
		fi, statErr := os.Stat(path)
		if statErr != nil {
			// A missing optional root is not fatal; access remains denied.
			return nil
		}
		if !fi.IsDir() {
			// Regular files cannot receive directory-only rights.
			access &^= uint64(unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
				unix.LANDLOCK_ACCESS_FS_REMOVE_FILE | unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
				unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_REG |
				unix.LANDLOCK_ACCESS_FS_MAKE_SOCK | unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
				unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK | unix.LANDLOCK_ACCESS_FS_MAKE_SYM |
				unix.LANDLOCK_ACCESS_FS_REFER)
		}
		pfd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil
		}
		defer unix.Close(pfd)
		pa := landlockPathBeneathAttr{AllowedAccess: access, ParentFd: int32(pfd)}
		_, _, errno := unix.Syscall(sysLandlockAddRule,
			uintptr(rulesetFd), landlockRuleTypePathBeneath, uintptr(unsafe.Pointer(&pa)))
		if errno != 0 && errno != unix.EEXIST {
			return fmt.Errorf("landlock_add_rule %s: %w", path, errno)
		}
		return nil
	}

	for _, r := range p.ReadOnlyRoots {
		if err := add(r, landlockReadOnly()); err != nil {
			return err
		}
	}
	for _, r := range p.ReadWriteRoots {
		if err := add(r, handled); err != nil {
			return err
		}
	}

	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl no_new_privs: %w", err)
	}
	_, _, errno = unix.Syscall(sysLandlockRestrictSelf, uintptr(rulesetFd), 0, 0)
	if errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", errno)
	}
	return nil
}

// systemReadOnlyRoots are the read-only roots required for dynamic linking,
// TLS trust, timezone and terminal data. Host secrets are not among them.
func systemReadOnlyRoots() []string {
	var roots []string
	for _, p := range []string{"/usr", "/lib", "/lib64", "/bin", "/sbin", "/etc/ld.so.cache",
		"/etc/ld.so.conf", "/etc/ld.so.conf.d", "/etc/resolv.conf", "/etc/hosts",
		"/etc/nsswitch.conf", "/etc/ssl", "/etc/pki", "/etc/ca-certificates",
		"/etc/alternatives", "/etc/terminfo", "/usr/share/terminfo", "/etc/localtime"} {
		if _, err := os.Stat(p); err == nil {
			roots = append(roots, p)
		}
	}
	return roots
}

// systemDeviceRoots are the writable device nodes a process needs. They are
// granted read-write; ordinary /dev entries are not exposed.
func systemDeviceRoots() []string {
	var roots []string
	for _, p := range []string{"/dev/null", "/dev/zero", "/dev/urandom", "/dev/random", "/dev/tty"} {
		if _, err := os.Stat(p); err == nil {
			roots = append(roots, p)
		}
	}
	return roots
}