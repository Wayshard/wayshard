//go:build linux

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"syscall"

	"github.com/Wayshard/wayshard/internal/sandbox"
	"golang.org/x/sys/unix"
)

// NewUserNetNSAttr returns the process attributes that create a fresh user and
// network namespace for the provider shim. The user namespace grants the shim
// CAP_NET_ADMIN over its own network namespace (so it can raise loopback)
// without root, and the network namespace contains only that loopback.
func NewUserNetNSAttr() (*syscall.SysProcAttr, error) {
	if os.Geteuid() == 0 {
		// Root does not need a user namespace to create a network namespace.
		return &syscall.SysProcAttr{
			Setpgid:    true,
			Pdeathsig:  syscall.SIGKILL,
			Cloneflags: unix.CLONE_NEWNET,
		}, nil
	}
	return &syscall.SysProcAttr{
		Setpgid:                    true,
		Pdeathsig:                  syscall.SIGKILL,
		Cloneflags:                 unix.CLONE_NEWUSER | unix.CLONE_NEWNET,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Geteuid(), Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getegid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
	}, nil
}

// ShimMain runs inside the created network namespace. It raises loopback,
// listens on the loopback proxy port, launches the harness under the caller's
// sandbox policy (Network=provider, inherited by exec), and bridges harness TCP
// to the host-side broker over a private Unix socket.
func ShimMain(configPath string) int {
	b, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-provider-shim: read config:", err)
		return 2
	}
	_ = os.Remove(configPath)
	var cfg ShimConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-provider-shim: parse config:", err)
		return 2
	}
	if cfg.BrokerSocket == "" || cfg.Bearer == "" || cfg.ProxyPort <= 0 {
		fmt.Fprintln(os.Stderr, "wayshard-provider-shim: incomplete config")
		return 2
	}
	if err := setLinkUp("lo"); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-provider-shim: loopback:", err)
		return 3
	}
	ln, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", cfg.ProxyPort))
	if err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-provider-shim: listen:", err)
		return 4
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go serveProxy(ctx, ln, cfg.BrokerSocket, cfg.Bearer)

	cmd := exec.Command(cfg.HarnessCommand, cfg.HarnessArgs...)
	cmd.Env = os.Environ()
	cmd.Dir = cfg.HarnessDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	con := sandbox.AsConstrainer(sandbox.DefaultBackend())
	if _, err := con.Compile(cfg.Policy); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-provider-shim: compile policy:", err)
		return 5
	}
	if err := con.Constrain(cmd, cfg.Policy); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-provider-shim: constrain harness:", err)
		return 5
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "wayshard-provider-shim: start harness:", err)
		return 6
	}
	werr := cmd.Wait()
	cancel()
	if werr == nil {
		return 0
	}
	if ee, ok := werr.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 7
}

func serveProxy(ctx context.Context, ln net.Listener, brokerSocket, bearer string) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			u, err := net.Dial("unix", brokerSocket)
			if err != nil {
				return
			}
			defer u.Close()
			if _, err := io.WriteString(u, bearer+"\n"); err != nil {
				return
			}
			go func() {
				_, _ = io.Copy(u, c)
				if uw, ok := u.(*net.UnixConn); ok {
					_ = uw.CloseWrite()
				}
			}()
			_, _ = io.Copy(c, u)
		}()
	}
}
