//go:build linux

package provider

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/unix"
)

const (
	rtmNewLink   = 16
	nlmsgError   = 2
	iflaIfname   = 3
	nlmsgAlignTo = 4
)

func align4(n int) int { return (n + nlmsgAlignTo - 1) &^ (nlmsgAlignTo - 1) }

func iflaAttr(typ uint16, data []byte) []byte {
	l := 4 + len(data)
	b := make([]byte, align4(l))
	binary.LittleEndian.PutUint16(b[0:], uint16(l))
	binary.LittleEndian.PutUint16(b[2:], typ)
	copy(b[4:], data)
	return b
}

// setLinkUp brings a named network interface up inside the current network
// namespace using netlink. It is used to raise the loopback interface in the
// per-attempt namespace so the broker shim can listen on 127.0.0.1.
func setLinkUp(name string) error {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return fmt.Errorf("netlink socket: %w", err)
	}
	defer unix.Close(fd)

	const seq = 1
	// struct ifinfomsg { __u8 family; __u8 pad; __u16 type; __s32 index; __u32 flags; __u32 change; }
	ifm := make([]byte, 16)
	binary.LittleEndian.PutUint32(ifm[8:], unix.IFF_UP)
	binary.LittleEndian.PutUint32(ifm[12:], unix.IFF_UP)
	payload := append(ifm, iflaAttr(iflaIfname, append([]byte(name), 0))...)

	msg := make([]byte, 16+len(payload))
	binary.LittleEndian.PutUint32(msg[0:], uint32(len(msg)))
	binary.LittleEndian.PutUint16(msg[4:], rtmNewLink)
	binary.LittleEndian.PutUint16(msg[6:], unix.NLM_F_REQUEST|unix.NLM_F_ACK)
	binary.LittleEndian.PutUint32(msg[8:], seq)
	copy(msg[16:], payload)

	if err := unix.Sendto(fd, msg, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return fmt.Errorf("netlink send: %w", err)
	}
	buf := make([]byte, 4096)
	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			return fmt.Errorf("netlink recv: %w", err)
		}
		for off := 0; off+16 <= n; {
			msgLen := int(binary.LittleEndian.Uint32(buf[off:]))
			typ := binary.LittleEndian.Uint16(buf[off+4:])
			if msgLen < 16 || off+msgLen > n {
				return fmt.Errorf("netlink malformed reply")
			}
			if typ == nlmsgError {
				if msgLen < 20 {
					return fmt.Errorf("netlink truncated error")
				}
				code := int32(binary.LittleEndian.Uint32(buf[off+16:]))
				if code == 0 {
					return nil
				}
				return fmt.Errorf("netlink set %s up: %w", name, unix.Errno(-code))
			}
			off += align4(msgLen)
		}
	}
}
