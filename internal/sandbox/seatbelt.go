package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SeatbeltProfile compiles SandboxPolicy into a macOS sandbox-exec profile.
// This is pure policy compilation and is tested on every OS.
func SeatbeltProfile(p Policy) (string, error) {
	if err := compileCommon(p); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	b.WriteString("(allow process*)\n")
	b.WriteString("(allow signal)\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow mach-lookup)\n")
	b.WriteString("(allow file-read-metadata)\n")
	b.WriteString("(allow file-read* (subpath \"/usr\") (subpath \"/bin\") (subpath \"/sbin\") (subpath \"/opt\") (subpath \"/private/etc\") (subpath \"/dev\"))\n")
	for _, root := range p.ReadOnlyRoots {
		if root == "" {
			continue
		}
		fmt.Fprintf(&b, "(allow file-read* (subpath %q))\n", filepath.Clean(root))
	}
	for _, root := range p.ReadWriteRoots {
		if root == "" {
			continue
		}
		fmt.Fprintf(&b, "(allow file-read* file-write* (subpath %q))\n", filepath.Clean(root))
	}
	if p.SyntheticHome != "" {
		fmt.Fprintf(&b, "(allow file-read* file-write* (subpath %q))\n", filepath.Clean(p.SyntheticHome))
	}
	if p.SyntheticTemp != "" {
		fmt.Fprintf(&b, "(allow file-read* file-write* (subpath %q))\n", filepath.Clean(p.SyntheticTemp))
	}
	switch p.Network {
	case NetNone, "":
		b.WriteString("(deny network*)\n")
	case NetAllowlist, NetBrokered:
		b.WriteString("(deny network*)\n")
		// allowlist destinations are brokered outside the profile
	case NetUnrestricted:
		b.WriteString("(allow network*)\n")
	}
	return b.String(), nil
}
