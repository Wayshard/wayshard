package sandbox

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// SeatbeltProfile compiles SandboxPolicy into a macOS sandbox-exec profile.
// This is pure policy compilation and is tested on every OS.
func SeatbeltProfile(p Policy) (string, error) {
	if err := compileCommon(p, false); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	// Apple's system.sb grants the process-exec/dyld/mach allowances needed to
	// start a binary. Write and network remain denied unless policy adds them.
	b.WriteString("(import \"system.sb\")\n")
	b.WriteString("(allow process-exec)\n")
	b.WriteString("(allow process-fork)\n")
	b.WriteString("(allow signal)\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow mach-lookup)\n")
	b.WriteString("(allow file-read-metadata)\n")
	b.WriteString("(allow file-map-executable)\n")
	b.WriteString("(allow file-ioctl)\n")
	b.WriteString("(allow ipc-posix-shm)\n")
	b.WriteString("(allow file-read* (literal \"/dev/dtracehelper\"))\n")
	b.WriteString("(allow file-ioctl (literal \"/dev/dtracehelper\"))\n")
	b.WriteString("(allow file-read* (literal \"/usr/lib/dyld\"))\n")
	b.WriteString("(allow file-read* (subpath \"/usr\") (subpath \"/bin\") (subpath \"/sbin\") (subpath \"/opt\") (subpath \"/System\") (subpath \"/Library\") (subpath \"/private/etc\") (subpath \"/private/var/db\") (subpath \"/dev\") (subpath \"/private/tmp\") (subpath \"/tmp\"))\n")
	for _, root := range p.ReadOnlyRoots {
		writeSeatbeltRoots(&b, root, false)
	}
	for _, root := range p.ReadWriteRoots {
		writeSeatbeltRoots(&b, root, true)
	}
	if p.SyntheticHome != "" {
		writeSeatbeltRoots(&b, p.SyntheticHome, true)
	}
	if p.SyntheticTemp != "" {
		writeSeatbeltRoots(&b, p.SyntheticTemp, true)
	}
	switch p.Network {
	case NetNone, "":
		b.WriteString("(deny network*)\n")
	case NetAllowlist, NetBrokered:
		b.WriteString("(deny network*)\n")
	case NetUnrestricted:
		b.WriteString("(allow network*)\n")
	}
	return b.String(), nil
}

func writeSeatbeltRoots(b *strings.Builder, root string, write bool) {
	for _, p := range seatbeltPaths(root) {
		if write {
			fmt.Fprintf(b, "(allow file-read* file-write* (subpath %s))\n", strconv.Quote(p))
		} else {
			fmt.Fprintf(b, "(allow file-read* (subpath %s))\n", strconv.Quote(p))
		}
	}
}

func seatbeltPaths(root string) []string {
	if root == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = filepath.Clean(p)
		if p == "" || p == "." || p == string(filepath.Separator) {
			return
		}
		p = filepath.ToSlash(p)
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	add(root)
	if rp, err := filepath.EvalSymlinks(root); err == nil {
		add(rp)
	}
	return out
}
