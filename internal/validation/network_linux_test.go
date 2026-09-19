//go:build linux

package validation

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/sandbox"
)

// TestValidationNetworkNoneEnforced proves the production validation runner
// applies NetworkNone to the checks it executes.
func TestValidationNetworkNoneEnforced(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	ws := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
/usr/bin/python3 - <<'PY'
import socket
def res(fn,label):
    try: fn(); print(label+"=OK")
    except OSError as e: print(label+"=ERRNO"+str(e.errno))
res(lambda: socket.socket(socket.AF_INET,socket.SOCK_STREAM).connect(("127.0.0.1",%s)),"TCP")
res(lambda: socket.socket(socket.AF_INET,socket.SOCK_DGRAM).sendto(b"x",("127.0.0.1",9)),"UDP")
res(lambda: socket.socket(socket.AF_UNIX,socket.SOCK_STREAM).connect("/tmp/nonexistent-ws.sock"),"UNIX")
PY
`, port)
	if err := os.WriteFile(filepath.Join(ws, "probe.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runner{DataDir: t.TempDir(), Network: sandbox.NetNone}
	art := r.Run(context.Background(), ws, []artifacts.ValidationCheck{
		{Name: "netnone", Command: "./probe.sh", Required: true, Status: string(domain.CheckNotVerified)},
	}, nil)
	if art.Checks[0].Status != string(domain.CheckPass) {
		t.Fatalf("validation network-none not enforced: %+v", art.Checks[0])
	}
}
