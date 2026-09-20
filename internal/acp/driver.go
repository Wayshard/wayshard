package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wayshard/wayshard/internal/version"
)

var (
	ErrFrameTooLarge    = errors.New("acp frame exceeds size limit")
	ErrTooManyPending   = errors.New("acp pending request limit exceeded")
	ErrProtocol         = errors.New("acp protocol error")
	ErrIncompatible     = errors.New("acp agent is not compatible with protocol v1")
	ErrClosed           = errors.New("acp driver closed")
	ErrStdoutPollution  = errors.New("acp stdout is not protocol JSON")
	ErrProcessExited    = errors.New("acp process exited")
	ErrHandshakeTimeout = errors.New("acp initialize handshake timed out")
)

const (
	defaultMaxFrameBytes    = 8 << 20
	defaultMaxPending       = 64
	defaultMaxStderrRetain  = 64 << 10
	defaultHandshakeTimeout = 15 * time.Second
	defaultShutdownWait     = 2 * time.Second
)

// Limits bound memory and in-flight RPC.
type Limits struct {
	MaxFrameBytes      int
	MaxPendingRequests int
	MaxStderrBytes     int
	HandshakeTimeout   time.Duration
	ShutdownWait       time.Duration
}

func DefaultLimits() Limits {
	return Limits{
		MaxFrameBytes:      defaultMaxFrameBytes,
		MaxPendingRequests: defaultMaxPending,
		MaxStderrBytes:     defaultMaxStderrRetain,
		HandshakeTimeout:   defaultHandshakeTimeout,
		ShutdownWait:       defaultShutdownWait,
	}
}

func (l Limits) withDefaults() Limits {
	d := DefaultLimits()
	if l.MaxFrameBytes <= 0 {
		l.MaxFrameBytes = d.MaxFrameBytes
	}
	if l.MaxPendingRequests <= 0 {
		l.MaxPendingRequests = d.MaxPendingRequests
	}
	if l.MaxStderrBytes <= 0 {
		l.MaxStderrBytes = d.MaxStderrBytes
	}
	if l.HandshakeTimeout <= 0 {
		l.HandshakeTimeout = d.HandshakeTimeout
	}
	if l.ShutdownWait <= 0 {
		l.ShutdownWait = d.ShutdownWait
	}
	return l
}

// Spec launches one ACP process. Stdout is protocol-only; stderr is diagnostic.
type Spec struct {
	Command    string
	Args       []string
	Env        []string
	Dir        string
	SetupCmd   func(*exec.Cmd) error
	AfterStart func(*exec.Cmd) (func(), error)
}

// Hooks are agent→client callbacks. Nil handlers deny or ignore.
type Hooks struct {
	OnEvent      func(Event)
	OnDiagnostic func(Diagnostic)

	RequestPermission func(ctx context.Context, p RequestPermissionParams) (RequestPermissionResult, error)
	ReadTextFile      func(ctx context.Context, p ReadTextFileParams) (ReadTextFileResult, error)
	WriteTextFile     func(ctx context.Context, p WriteTextFileParams) (WriteTextFileResult, error)
	CreateTerminal    func(ctx context.Context, p CreateTerminalParams) (CreateTerminalResult, error)
	TerminalOutput    func(ctx context.Context, p TerminalIDParams) (TerminalOutputResult, error)
	ReleaseTerminal   func(ctx context.Context, p TerminalIDParams) error
	WaitTerminalExit  func(ctx context.Context, p TerminalIDParams) (WaitForTerminalExitResult, error)
	KillTerminal      func(ctx context.Context, p TerminalIDParams) error
}

// ClientConfig is advertised during initialize.
type ClientConfig struct {
	Info         Implementation
	Capabilities ClientCapabilities
}

func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Info: Implementation{
			Name:    "wayshard",
			Title:   version.Product,
			Version: version.Version,
		},
		Capabilities: ClientCapabilities{
			FS: FileSystemCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
	}
}

type rpcResult struct {
	resp Response
	err  error
}

type pendingCall struct {
	ch     chan rpcResult
	method string
}

// Driver is an ACP v1 protocol driver for one agent process.
type Driver struct {
	spec   Spec
	limits Limits
	hooks  Hooks
	client ClientConfig

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	mu       sync.Mutex
	writeMu  sync.Mutex
	nextID   atomic.Int64
	pending  map[string]*pendingCall
	inflight atomic.Int32

	init       *InitializeResponse
	closed     atomic.Bool
	exitErr    error
	dead       chan struct{}
	lifetime   context.Context
	cancel     context.CancelFunc
	extraClean func()

	diagMu       sync.Mutex
	diagnostics  []Diagnostic
	stderrBytes  int
	messageParts []string
}

// Launch starts the ACP process. Handshake is separate so probes can bound it.
func Launch(parent context.Context, spec Spec, client ClientConfig, hooks Hooks, limits Limits) (*Driver, error) {
	if spec.Command == "" {
		return nil, fmt.Errorf("%w: empty command", ErrProtocol)
	}
	limits = limits.withDefaults()
	if client.Info.Name == "" {
		client = DefaultClientConfig()
	}
	ctx, cancel := context.WithCancel(parent)
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.Dir
	if spec.Env != nil {
		cmd.Env = sanitizeEnv(spec.Env)
	} else {
		cmd.Env = sanitizeEnv(os.Environ())
	}
	setProcAttr(cmd)
	if spec.SetupCmd != nil {
		if err := spec.SetupCmd(cmd); err != nil {
			cancel()
			return nil, err
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("launch %s: %w", spec.Command, err)
	}
	var extraClean func()
	if spec.AfterStart != nil {
		cfn, err := spec.AfterStart(cmd)
		if err != nil {
			_ = cmd.Process.Kill()
			cancel()
			return nil, err
		}
		extraClean = cfn
	}
	d := &Driver{
		spec:       spec,
		limits:     limits,
		hooks:      hooks,
		client:     client,
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		pending:    make(map[string]*pendingCall),
		dead:       make(chan struct{}),
		lifetime:   ctx,
		cancel:     cancel,
		extraClean: extraClean,
	}
	go d.readStdout()
	go d.readStderr()
	go d.waitProc()
	go func() {
		<-ctx.Done()
		d.shutdown()
	}()
	return d, nil
}

// Handshake sends initialize (protocolVersion 1) and records agent capabilities.
func (d *Driver) Handshake(ctx context.Context) (*InitializeResponse, error) {
	if d.limits.HandshakeTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.limits.HandshakeTimeout)
		defer cancel()
	}
	req := InitializeRequest{
		ProtocolVersion:    ProtocolVersion(ProtocolVersionV1),
		ClientCapabilities: d.client.Capabilities,
		ClientInfo:         d.client.Info,
	}
	var resp InitializeResponse
	if err := d.call(ctx, MethodInitialize, req, &resp); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrHandshakeTimeout, err)
		}
		return nil, err
	}
	if int(resp.ProtocolVersion) != ProtocolVersionV1 {
		return &resp, fmt.Errorf("%w: agent selected protocolVersion %d", ErrIncompatible, resp.ProtocolVersion)
	}
	d.mu.Lock()
	d.init = &resp
	d.mu.Unlock()
	return &resp, nil
}

func (d *Driver) Capabilities() AgentCapabilities {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.init == nil {
		return AgentCapabilities{}
	}
	return d.init.AgentCapabilities
}

func (d *Driver) InitializeResult() *InitializeResponse {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.init
}

func (d *Driver) Authenticate(ctx context.Context, req AuthenticateRequest) (*AuthenticateResponse, error) {
	var resp AuthenticateResponse
	if err := d.call(ctx, MethodAuthenticate, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Driver) NewSession(ctx context.Context, req NewSessionRequest) (*NewSessionResponse, error) {
	if req.MCPServers == nil {
		req.MCPServers = []MCPServer{}
	}
	var resp NewSessionResponse
	if err := d.call(ctx, MethodSessionNew, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SetConfigOption sets an agent session config option (for example the "model"
// option advertised by some agents). It returns an error when the agent does
// not support the method.
func (d *Driver) SetConfigOption(ctx context.Context, sessionID, configID, value string) error {
	var resp json.RawMessage
	return d.call(ctx, "session/set_config_option", map[string]any{
		"sessionId": sessionID, "configId": configID, "value": value,
	}, &resp)
}

// SetModel sets the session model using the method used by agents such as Codex.
func (d *Driver) SetModel(ctx context.Context, sessionID, modelID string) error {
	var resp json.RawMessage
	return d.call(ctx, "session/set_model", map[string]any{
		"sessionId": sessionID, "modelId": modelID,
	}, &resp)
}

func (d *Driver) LoadSession(ctx context.Context, req LoadSessionRequest) (*LoadSessionResponse, error) {
	if req.MCPServers == nil {
		req.MCPServers = []MCPServer{}
	}
	var resp LoadSessionResponse
	if err := d.call(ctx, MethodSessionLoad, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Prompt runs session/prompt and streams normalized events via Hooks.OnEvent.
func (d *Driver) Prompt(ctx context.Context, req PromptRequest) (*PromptResponse, error) {
	d.mu.Lock()
	d.messageParts = d.messageParts[:0]
	d.mu.Unlock()
	var resp PromptResponse
	if err := d.call(ctx, MethodSessionPrompt, req, &resp); err != nil {
		d.emit(NormalizeError(req.SessionID, err.Error()))
		return nil, err
	}
	d.emit(NormalizeDone(req.SessionID, resp))
	return &resp, nil
}

func (d *Driver) MessageText() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return strings.Join(d.messageParts, "")
}

// Cancel sends session/cancel. It is a notification; no response is expected.
func (d *Driver) Cancel(sessionID string) error {
	return d.notify(Notification{
		JSONRPC: JSONRPCVersion,
		Method:  MethodSessionCancel,
		Params:  mustJSON(CancelNotification{SessionID: sessionID}),
	})
}

func (d *Driver) PID() int {
	if d.cmd == nil || d.cmd.Process == nil {
		return 0
	}
	return d.cmd.Process.Pid
}

func (d *Driver) Diagnostics() []Diagnostic {
	d.diagMu.Lock()
	defer d.diagMu.Unlock()
	out := make([]Diagnostic, len(d.diagnostics))
	copy(out, d.diagnostics)
	return out
}

func (d *Driver) Close() error {
	d.shutdown()
	select {
	case <-d.dead:
	case <-time.After(d.limits.ShutdownWait + time.Second):
	}
	d.mu.Lock()
	err := d.exitErr
	d.mu.Unlock()
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return err
	}
	return err
}

func (d *Driver) shutdown() {
	if !d.closed.CompareAndSwap(false, true) {
		return
	}
	d.cancel()
	d.writeMu.Lock()
	if d.stdin != nil {
		_ = d.stdin.Close()
	}
	d.writeMu.Unlock()
	select {
	case <-d.dead:
		return
	default:
	}
	if d.cmd != nil && d.cmd.Process != nil {
		pid := d.cmd.Process.Pid
		_ = signalTerm(pid)
		select {
		case <-d.dead:
			if d.extraClean != nil {
				d.extraClean()
			}
			return
		case <-time.After(d.limits.ShutdownWait):
			select {
			case <-d.dead:
			default:
				_ = killTree(pid)
			}
		}
	}
	if d.extraClean != nil {
		d.extraClean()
	}
}

func (d *Driver) waitProc() {
	err := d.cmd.Wait()
	d.mu.Lock()
	d.exitErr = err
	d.mu.Unlock()
	close(d.dead)
	d.failAll(fmt.Errorf("%w: %v", ErrProcessExited, err))
}

func (d *Driver) failAll(err error) {
	d.mu.Lock()
	pending := d.pending
	d.pending = make(map[string]*pendingCall)
	d.mu.Unlock()
	for _, p := range pending {
		p.ch <- rpcResult{err: err}
	}
}

func (d *Driver) call(ctx context.Context, method string, params any, out any) error {
	if d.closed.Load() {
		return ErrClosed
	}
	select {
	case <-d.dead:
		d.mu.Lock()
		err := d.exitErr
		d.mu.Unlock()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrProcessExited, err)
		}
		return ErrClosed
	default:
	}
	if int(d.inflight.Load()) >= d.limits.MaxPendingRequests {
		return ErrTooManyPending
	}
	id := NewNumberID(d.nextID.Add(1))
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	ch := make(chan rpcResult, 1)
	key := id.String()
	d.mu.Lock()
	d.pending[key] = &pendingCall{ch: ch, method: method}
	d.mu.Unlock()
	d.inflight.Add(1)
	defer d.inflight.Add(-1)

	req := Request{JSONRPC: JSONRPCVersion, ID: id, Method: method, Params: b}
	if err := d.write(req); err != nil {
		d.dropPending(key)
		return err
	}
	select {
	case res := <-ch:
		if res.err != nil {
			return res.err
		}
		resp := res.resp
		if resp.Error != nil {
			return resp.Error
		}
		if out != nil && len(resp.Result) > 0 && string(resp.Result) != "null" {
			if err := json.Unmarshal(resp.Result, out); err != nil {
				return fmt.Errorf("%w: decode %s result: %v", ErrProtocol, method, err)
			}
		}
		return nil
	case <-ctx.Done():
		d.dropPending(key)
		return ctx.Err()
	case <-d.lifetime.Done():
		d.dropPending(key)
		return ErrClosed
	case <-d.dead:
		d.dropPending(key)
		d.mu.Lock()
		err := d.exitErr
		d.mu.Unlock()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrProcessExited, err)
		}
		return ErrClosed
	}
}

func (d *Driver) dropPending(key string) {
	d.mu.Lock()
	delete(d.pending, key)
	d.mu.Unlock()
}

func (d *Driver) notify(n Notification) error {
	if n.JSONRPC == "" {
		n.JSONRPC = JSONRPCVersion
	}
	return d.write(n)
}

func (d *Driver) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if bytes.Contains(b, []byte{'\n'}) {
		return fmt.Errorf("%w: marshaled frame contains newline", ErrProtocol)
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	if d.closed.Load() {
		return ErrClosed
	}
	_, err = d.stdin.Write(append(b, '\n'))
	return err
}

func (d *Driver) readStdout() {
	br := bufio.NewReaderSize(d.stdout, 64*1024)
	for {
		line, err := readFrame(br, d.limits.MaxFrameBytes)
		if len(line) > 0 {
			d.handleFrame(line)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !d.closed.Load() {
				d.recordDiag(Diagnostic{Source: "protocol", Text: Redact(err.Error())})
				if errors.Is(err, ErrFrameTooLarge) {
					d.failAll(err)
					d.shutdown()
				}
			}
			return
		}
	}
}

func (d *Driver) handleFrame(line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}
	if line[0] != '{' && line[0] != '[' {
		d.recordDiag(Diagnostic{Source: "stdout_pollution", Text: Redact(string(line))})
		d.emit(Event{
			Kind:       EventError,
			Error:      ErrStdoutPollution.Error(),
			Diagnostic: &Diagnostic{Source: "stdout_pollution", Text: Redact(string(line))},
		})
		d.failAll(fmt.Errorf("%w: %s", ErrStdoutPollution, Bound(string(line), 200)))
		return
	}
	// Unknown batch arrays: tolerate by handling each object.
	if line[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(line, &items); err != nil {
			d.protocolFail(line, err)
			return
		}
		for _, it := range items {
			d.handleFrame(it)
		}
		return
	}
	var w Wire
	if err := json.Unmarshal(line, &w); err != nil {
		d.protocolFail(line, err)
		return
	}
	switch {
	case w.IsResponse():
		d.handleResponse(w.Response())
	case w.IsNotification():
		d.handleNotification(w.Notification())
	case w.IsRequest():
		if int(d.inflight.Load()) >= d.limits.MaxPendingRequests {
			_ = d.write(Response{
				JSONRPC: JSONRPCVersion,
				ID:      *w.ID,
				Error:   NewError(ErrCodeTooManyPending, ErrTooManyPending.Error()),
			})
			return
		}
		d.inflight.Add(1)
		go func() {
			defer d.inflight.Add(-1)
			d.handleAgentRequest(w.Request())
		}()
	default:
		d.recordDiag(Diagnostic{Source: "protocol", Text: Redact("ignored malformed json-rpc frame")})
	}
}

func (d *Driver) protocolFail(line []byte, err error) {
	d.recordDiag(Diagnostic{Source: "protocol", Text: Redact(err.Error() + " " + string(line))})
	d.emit(NormalizeError("", err.Error()))
	d.failAll(fmt.Errorf("%w: %v", ErrProtocol, err))
}

func (d *Driver) handleResponse(resp Response) {
	key := resp.ID.String()
	d.mu.Lock()
	p, ok := d.pending[key]
	if ok {
		delete(d.pending, key)
	}
	d.mu.Unlock()
	if !ok {
		d.recordDiag(Diagnostic{Source: "protocol", Text: "unexpected response id " + key})
		return
	}
	p.ch <- rpcResult{resp: resp}
}

func (d *Driver) handleNotification(n Notification) {
	switch n.Method {
	case MethodSessionUpdate:
		var sn SessionNotification
		if err := json.Unmarshal(n.Params, &sn); err != nil {
			d.recordDiag(Diagnostic{Source: "protocol", Text: Redact("session/update: " + err.Error())})
			return
		}
		for _, ev := range NormalizeUpdate(sn) {
			if ev.Kind == EventMessageDelta && ev.Text != "" {
				d.mu.Lock()
				d.messageParts = append(d.messageParts, ev.Text)
				d.mu.Unlock()
			}
			if ev.Diagnostic != nil {
				d.recordDiag(*ev.Diagnostic)
				if ev.Kind == EventError && ev.Error == "" {
					continue // unknown extension: diagnostic only
				}
			}
			d.emit(ev)
		}
	default:
		// Unknown notifications, including _extensions, are tolerated.
		d.recordDiag(Diagnostic{Source: "extension", Text: Redact("notification " + n.Method)})
	}
}

func (d *Driver) handleAgentRequest(req Request) {
	ctx := d.lifetime
	var (
		result any
		rpcErr *Error
	)
	switch req.Method {
	case MethodSessionRequestPermission:
		var p RequestPermissionParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			rpcErr = NewError(ErrCodeInvalidParams, err.Error())
			break
		}
		d.emit(NormalizePermission(p))
		if d.hooks.RequestPermission == nil {
			result = CancelledPermission()
			break
		}
		out, err := d.hooks.RequestPermission(ctx, p)
		if err != nil {
			rpcErr = NewError(ErrCodeInternal, err.Error())
			break
		}
		result = out
	case MethodFSReadTextFile:
		var p ReadTextFileParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			rpcErr = NewError(ErrCodeInvalidParams, err.Error())
			break
		}
		if d.hooks.ReadTextFile == nil {
			rpcErr = NewError(ErrCodeMethodNotFound, "fs/read_text_file not available")
			break
		}
		out, err := d.hooks.ReadTextFile(ctx, p)
		if err != nil {
			rpcErr = asRPCError(err)
			break
		}
		result = out
	case MethodFSWriteTextFile:
		var p WriteTextFileParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			rpcErr = NewError(ErrCodeInvalidParams, err.Error())
			break
		}
		if d.hooks.WriteTextFile == nil {
			rpcErr = NewError(ErrCodeMethodNotFound, "fs/write_text_file not available")
			break
		}
		out, err := d.hooks.WriteTextFile(ctx, p)
		if err != nil {
			rpcErr = asRPCError(err)
			break
		}
		result = out
	case MethodTerminalCreate:
		var p CreateTerminalParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			rpcErr = NewError(ErrCodeInvalidParams, err.Error())
			break
		}
		if d.hooks.CreateTerminal == nil {
			rpcErr = NewError(ErrCodeMethodNotFound, "terminal/create not available")
			break
		}
		out, err := d.hooks.CreateTerminal(ctx, p)
		if err != nil {
			rpcErr = asRPCError(err)
			break
		}
		result = out
	case MethodTerminalOutput:
		var p TerminalIDParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			rpcErr = NewError(ErrCodeInvalidParams, err.Error())
			break
		}
		if d.hooks.TerminalOutput == nil {
			rpcErr = NewError(ErrCodeMethodNotFound, "terminal/output not available")
			break
		}
		out, err := d.hooks.TerminalOutput(ctx, p)
		if err != nil {
			rpcErr = asRPCError(err)
			break
		}
		result = out
	case MethodTerminalRelease:
		var p TerminalIDParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			rpcErr = NewError(ErrCodeInvalidParams, err.Error())
			break
		}
		if d.hooks.ReleaseTerminal == nil {
			rpcErr = NewError(ErrCodeMethodNotFound, "terminal/release not available")
			break
		}
		if err := d.hooks.ReleaseTerminal(ctx, p); err != nil {
			rpcErr = asRPCError(err)
			break
		}
		result = map[string]any{}
	case MethodTerminalWaitForExit:
		var p TerminalIDParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			rpcErr = NewError(ErrCodeInvalidParams, err.Error())
			break
		}
		if d.hooks.WaitTerminalExit == nil {
			rpcErr = NewError(ErrCodeMethodNotFound, "terminal/wait_for_exit not available")
			break
		}
		out, err := d.hooks.WaitTerminalExit(ctx, p)
		if err != nil {
			rpcErr = asRPCError(err)
			break
		}
		result = out
	case MethodTerminalKill:
		var p TerminalIDParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			rpcErr = NewError(ErrCodeInvalidParams, err.Error())
			break
		}
		if d.hooks.KillTerminal == nil {
			rpcErr = NewError(ErrCodeMethodNotFound, "terminal/kill not available")
			break
		}
		if err := d.hooks.KillTerminal(ctx, p); err != nil {
			rpcErr = asRPCError(err)
			break
		}
		result = map[string]any{}
	default:
		// Unknown extension methods are tolerated with method-not-found.
		d.recordDiag(Diagnostic{Source: "extension", Text: Redact("agent request " + req.Method)})
		rpcErr = NewError(ErrCodeMethodNotFound, "method not found: "+req.Method)
	}
	resp := Response{JSONRPC: JSONRPCVersion, ID: req.ID}
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		resp.Result = mustJSON(result)
	}
	_ = d.write(resp)
}

func (d *Driver) readStderr() {
	sc := bufio.NewScanner(d.stderr)
	buf := make([]byte, 0, 8*1024)
	sc.Buffer(buf, 256*1024)
	for sc.Scan() {
		line := sc.Text()
		d.recordDiag(Diagnostic{Source: "stderr", Text: Redact(line)})
	}
}

func (d *Driver) emit(ev Event) {
	if d.hooks.OnEvent != nil {
		d.hooks.OnEvent(ev)
	}
}

func (d *Driver) recordDiag(diag Diagnostic) {
	if diag.Text == "" {
		return
	}
	diag.Text = Redact(diag.Text)
	if len(diag.Text) >= maxDiagnosticBytes {
		diag.Truncated = true
	}
	d.diagMu.Lock()
	d.stderrBytes += len(diag.Text)
	if len(d.diagnostics) >= maxDiagnostics || d.stderrBytes > d.limits.MaxStderrBytes {
		if len(d.diagnostics) > 0 {
			d.diagnostics = d.diagnostics[1:]
		}
		diag.Truncated = true
	}
	d.diagnostics = append(d.diagnostics, diag)
	d.diagMu.Unlock()
	if d.hooks.OnDiagnostic != nil {
		d.hooks.OnDiagnostic(diag)
	}
}

func asRPCError(err error) *Error {
	var rpc *Error
	if errors.As(err, &rpc) {
		return rpc
	}
	return NewError(ErrCodeInternal, err.Error())
}

func mustJSON(v any) json.RawMessage {
	if v == nil {
		return json.RawMessage("{}")
	}
	if raw, ok := v.(json.RawMessage); ok {
		return raw
	}
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

func readFrame(br *bufio.Reader, max int) ([]byte, error) {
	var out []byte
	for {
		chunk, err := br.ReadSlice('\n')
		if len(out)+len(chunk) > max {
			return nil, ErrFrameTooLarge
		}
		out = append(out, chunk...)
		if err == nil {
			out = bytes.TrimSuffix(out, []byte("\n"))
			out = bytes.TrimSuffix(out, []byte("\r"))
			return out, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(out) == 0 {
				return nil, io.EOF
			}
			return out, io.EOF
		}
		return nil, err
	}
}

func sanitizeEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		uk := strings.ToUpper(key)
		switch {
		case strings.HasPrefix(uk, "WAYSHARD_JEV"),
			strings.HasPrefix(uk, "WAYSHARD_VAULT"),
			uk == "JEV_API_KEY",
			strings.HasPrefix(uk, "TYPESAFE"):
			continue
		}
		out = append(out, e)
	}
	return out
}

// ProbeResult is a one-shot initialize probe of an installed executable.
type ProbeResult struct {
	Initialize *InitializeResponse
	Version    string
	Duration   time.Duration
	Stderr     []Diagnostic
}

// Probe launches the process, performs initialize, and tears it down.
func Probe(ctx context.Context, spec Spec, timeout time.Duration) (*ProbeResult, error) {
	if timeout <= 0 {
		timeout = defaultHandshakeTimeout
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d, err := Launch(ctx, spec, DefaultClientConfig(), Hooks{}, Limits{HandshakeTimeout: timeout})
	if err != nil {
		return nil, err
	}
	defer d.Close()
	init, err := d.Handshake(ctx)
	res := &ProbeResult{
		Initialize: init,
		Duration:   time.Since(start),
		Stderr:     d.Diagnostics(),
	}
	return res, err
}

// ProbeVersion runs command --version with a short timeout. Failure is non-fatal.
func ProbeVersion(ctx context.Context, command string, extraArgs []string) string {
	return ProbeVersionEnv(ctx, command, extraArgs, nil)
}

// ProbeVersionEnv is ProbeVersion with an explicit, already-confined environment.
func ProbeVersionEnv(ctx context.Context, command string, extraArgs, env []string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	args := append([]string{}, extraArgs...)
	args = append(args, "--version")
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdin = nil
	if env != nil {
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return strings.TrimSpace(string(bytes.SplitN(out, []byte("\n"), 2)[0]))
	}
	cmd = exec.CommandContext(ctx, command, "--version")
	cmd.Stdin = nil
	if env != nil {
		cmd.Env = env
	}
	out, err = cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(bytes.SplitN(out, []byte("\n"), 2)[0]))
}
