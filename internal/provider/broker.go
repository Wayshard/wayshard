package provider

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Broker is a per-attempt, destination-validating HTTPS CONNECT proxy. It
// listens on a private filesystem Unix socket that only the attempt's in-namespace
// shim can reach (the harness itself is denied AF_UNIX and cannot see the host
// filesystem), so it is not a generic same-user open proxy.
//
// Only CONNECT tunneling is supported. TLS stays end-to-end: the broker sees the
// destination metadata but never plaintext, and it never inspects or rewrites
// headers. Plain HTTP forwarding is refused rather than implemented with a
// weaker, redirect-sensitive path.
type Broker struct {
	socketPath string
	bearer     string
	policy     Policy
	resolver   Resolver
	dialer     Dialer
	log        *slog.Logger

	ln        net.Listener
	mu        sync.Mutex
	conns     map[net.Conn]struct{}
	closed    bool
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// StartBroker creates the Unix socket and serves connections until Close.
func StartBroker(socketPath, bearer string, policy Policy, resolver Resolver, dialer Dialer, log *slog.Logger) (*Broker, error) {
	if !policy.Eligible() {
		return nil, fmt.Errorf("provider broker requires at least one configured destination")
	}
	if bearer == "" {
		return nil, fmt.Errorf("provider broker requires a bearer capability")
	}
	if log == nil {
		log = slog.Default()
	}
	if resolver == nil {
		resolver = DefaultResolver{}
	}
	if dialer == nil {
		dialer = DefaultDialer{}
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return nil, err
	}
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("provider broker listen: %w", err)
	}
	b := &Broker{
		socketPath: socketPath, bearer: bearer, policy: policy,
		resolver: resolver, dialer: dialer, log: log,
		ln: ln, conns: map[net.Conn]struct{}{},
	}
	b.wg.Add(1)
	go b.serve()
	return b, nil
}

// SocketPath is the private Unix socket the in-namespace shim connects to.
func (b *Broker) SocketPath() string { return b.socketPath }

func (b *Broker) serve() {
	defer b.wg.Done()
	for {
		c, err := b.ln.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			_ = c.Close()
			return
		}
		b.conns[c] = struct{}{}
		b.mu.Unlock()
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			b.handle(c)
			b.mu.Lock()
			delete(b.conns, c)
			b.mu.Unlock()
		}()
	}
}

func (b *Broker) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))

	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil {
		return
	}
	if strings.TrimSpace(line) != b.bearer {
		writeStatus(c, http.StatusUnauthorized)
		return
	}
	req, err := http.ReadRequest(br)
	if err != nil {
		writeStatus(c, http.StatusBadRequest)
		return
	}
	if req.Method != http.MethodConnect {
		// Documented behavior: only CONNECT tunneling is supported.
		writeStatus(c, http.StatusMethodNotAllowed)
		return
	}
	host, port, err := splitConnectTarget(req.Host)
	if err != nil {
		writeStatus(c, http.StatusBadRequest)
		return
	}
	dest, err := b.policy.Authorize(host, port)
	if err != nil {
		b.log.Warn("provider destination refused", "host", host, "port", port, "reason", "not_authorized")
		writeStatus(c, http.StatusForbidden)
		return
	}
	// Revalidate on every connection: a hostname that later resolves to a
	// private/loopback address is refused (DNS rebinding).
	addr, err := ResolveValidated(context.Background(), b.resolver, dest.Host)
	if err != nil {
		b.log.Warn("provider destination refused", "host", dest.Host, "port", dest.Port, "reason", "disallowed_address")
		writeStatus(c, http.StatusForbidden)
		return
	}
	upstream, err := b.dialer.DialContext(context.Background(), "tcp", net.JoinHostPort(addr.String(), strconv.Itoa(dest.Port)))
	if err != nil {
		b.log.Warn("provider upstream dial failed", "host", dest.Host, "port", dest.Port, "reason", "dial_error")
		writeStatus(c, http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	_ = c.SetDeadline(time.Time{})
	_ = upstream.SetDeadline(time.Time{})
	if _, err := io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	b.log.Info("provider.connect", "host", dest.Host, "port", dest.Port, "address", addr.String())

	client := io.Reader(c)
	if n := br.Buffered(); n > 0 {
		peeked, _ := br.Peek(n)
		client = io.MultiReader(bytes.NewReader(peeked), c)
	}
	go func() {
		_, _ = io.Copy(upstream, client)
		if uc, ok := upstream.(interface{ CloseWrite() error }); ok {
			_ = uc.CloseWrite()
		}
	}()
	_, _ = io.Copy(c, upstream)
}

// Close closes the listener and every active tunnel.
func (b *Broker) Close() error {
	var err error
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.closed = true
		ln := b.ln
		b.mu.Unlock()
		if ln != nil {
			err = ln.Close()
		}
		b.mu.Lock()
		for c := range b.conns {
			_ = c.Close()
		}
		b.mu.Unlock()
		b.wg.Wait()
		_ = os.Remove(b.socketPath)
	})
	return err
}

const maxConnectTargetLen = 512

func splitConnectTarget(target string) (string, int, error) {
	if target == "" || len(target) > maxConnectTargetLen {
		return "", 0, fmt.Errorf("invalid CONNECT target")
	}
	if strings.ContainsAny(target, " \t\r\n") {
		return "", 0, fmt.Errorf("invalid CONNECT target")
	}
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		// No explicit port: default to HTTPS.
		host, portStr = target, "443"
	}
	if strings.Contains(host, "@") {
		return "", 0, fmt.Errorf("invalid CONNECT target")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("invalid CONNECT port")
	}
	return host, port, nil
}

func writeStatus(c net.Conn, code int) {
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = fmt.Fprintf(c, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\n\r\n", code, http.StatusText(code))
}
