package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/Wayshard/wayshard/internal/acp"
	"github.com/Wayshard/wayshard/internal/id"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/process"
	"github.com/Wayshard/wayshard/internal/sandbox"
)

// toolManager owns ACP terminal/tool processes requested by a harness. Every
// command runs through the Tool Sandbox (NetworkNone, workspace-scoped,
// synthetic HOME/TEMP, allowlisted env). Wayshard, not the harness, owns
// lifecycle: cancellation, timeout and release terminate the whole process tree.
type toolManager struct {
	mu        sync.Mutex
	sessions  map[string]*toolSession
	workspace string
	home      string
	token     string
	readOnly  bool
	backend   sandbox.Backend
	timeout   time.Duration
	outputCap int
}

type toolSession struct {
	id        string
	cmd       *exec.Cmd
	con       sandbox.Constrainer
	mu        sync.Mutex
	buf       bytes.Buffer
	truncated bool
	done      chan struct{}
	exitCode  *int
	signal    string
	limit     int
	cancel    context.CancelFunc
}

func newToolManager(req orchestrator.StageRequest, workspace, home string, backend sandbox.Backend, token string) *toolManager {
	if backend == nil {
		backend = sandbox.DefaultBackend()
	}
	return &toolManager{
		sessions:  map[string]*toolSession{},
		workspace: workspace,
		home:      home,
		token:     token,
		readOnly:  req.Stage.Kind.ReadOnly(),
		backend:   backend,
		timeout:   10 * time.Minute,
		outputCap: 1 << 20,
	}
}

func (m *toolManager) policy() sandbox.Policy {
	if m.readOnly {
		return sandbox.ReadOnlyViewPolicy(m.workspace, m.home)
	}
	return sandbox.ToolPolicy(m.workspace, m.home, sandbox.NetNone)
}

func (m *toolManager) Create(ctx context.Context, p acp.CreateTerminalParams) (acp.CreateTerminalResult, error) {
	if m.workspace == "" {
		return acp.CreateTerminalResult{}, fmt.Errorf("no run workspace for tool execution")
	}
	dir := m.workspace
	if p.CWD != "" {
		full, err := withinRoot(m.workspace, p.CWD)
		if err != nil {
			return acp.CreateTerminalResult{}, fmt.Errorf("cwd escapes run workspace")
		}
		dir = full
	}
	s := &toolSession{id: id.New(), done: make(chan struct{}), limit: m.outputCap}
	writer := &boundedWriter{s: s}
	cmd := exec.Command(p.Command, p.Args...)
	cmd.Dir = dir
	scoped := map[string]string{}
	if m.token != "" {
		scoped[process.TokenEnv] = m.token
	}
	cmd.Env = sandbox.ToolEnv(m.home, m.home, scoped)
	cmd.Stdout = writer
	cmd.Stderr = writer
	s.cmd = cmd

	con := sandbox.AsConstrainer(m.backend)
	pol := m.policy()
	if _, err := con.Compile(pol); err != nil {
		return acp.CreateTerminalResult{}, err
	}
	if err := con.Constrain(cmd, pol); err != nil {
		return acp.CreateTerminalResult{}, err
	}
	s.con = con
	if err := cmd.Start(); err != nil {
		return acp.CreateTerminalResult{}, err
	}
	if _, err := con.Attach(cmd, pol); err != nil {
		_ = con.KillTree(cmd)
		return acp.CreateTerminalResult{}, err
	}

	cctx, cancel := context.WithTimeout(ctx, m.timeout)
	s.cancel = cancel
	m.mu.Lock()
	m.sessions[s.id] = s
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		if cmd.ProcessState != nil {
			code := cmd.ProcessState.ExitCode()
			s.exitCode = &code
		}
		if err != nil {
			s.signal = "error"
		}
		s.mu.Unlock()
		close(s.done)
	}()
	go func() {
		select {
		case <-cctx.Done():
			_ = con.KillTree(cmd)
		case <-s.done:
		}
	}()
	return acp.CreateTerminalResult{TerminalID: s.id}, nil
}

type boundedWriter struct{ s *toolSession }

func (w *boundedWriter) Write(p []byte) (int, error) {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	remain := w.s.limit - w.s.buf.Len()
	if remain <= 0 {
		w.s.truncated = true
		return len(p), nil
	}
	if len(p) > remain {
		w.s.buf.Write(p[:remain])
		w.s.truncated = true
		return len(p), nil
	}
	w.s.buf.Write(p)
	return len(p), nil
}

func (m *toolManager) session(id string) (*toolSession, error) {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil {
		return nil, fmt.Errorf("unknown terminal %q", id)
	}
	return s, nil
}

func (m *toolManager) Output(_ context.Context, p acp.TerminalIDParams) (acp.TerminalOutputResult, error) {
	s, err := m.session(p.TerminalID)
	if err != nil {
		return acp.TerminalOutputResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res := acp.TerminalOutputResult{Output: s.buf.String(), Truncated: s.truncated}
	if s.exitCode != nil {
		if b, err := json.Marshal(map[string]any{"exitCode": *s.exitCode}); err == nil {
			res.ExitStatus = b
		}
	}
	return res, nil
}

func (m *toolManager) WaitExit(ctx context.Context, p acp.TerminalIDParams) (acp.WaitForTerminalExitResult, error) {
	s, err := m.session(p.TerminalID)
	if err != nil {
		return acp.WaitForTerminalExitResult{}, err
	}
	select {
	case <-s.done:
	case <-ctx.Done():
		return acp.WaitForTerminalExitResult{}, ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := acp.WaitForTerminalExitResult{ExitCode: s.exitCode}
	if s.signal != "" {
		sig := s.signal
		out.Signal = &sig
	}
	return out, nil
}

func (m *toolManager) Kill(_ context.Context, p acp.TerminalIDParams) error {
	s, err := m.session(p.TerminalID)
	if err != nil {
		return err
	}
	s.cancel()
	return s.con.KillTree(s.cmd)
}

func (m *toolManager) Release(_ context.Context, p acp.TerminalIDParams) error {
	m.mu.Lock()
	s := m.sessions[p.TerminalID]
	delete(m.sessions, p.TerminalID)
	m.mu.Unlock()
	if s != nil {
		s.cancel()
		_ = s.con.KillTree(s.cmd)
	}
	return nil
}

// CloseAll terminates every tool process tree owned by this manager.
func (m *toolManager) CloseAll() {
	m.mu.Lock()
	sessions := make([]*toolSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = map[string]*toolSession{}
	m.mu.Unlock()
	for _, s := range sessions {
		s.cancel()
		_ = s.con.KillTree(s.cmd)
	}
}
