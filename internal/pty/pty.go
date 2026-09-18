package pty

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/Wayshard/wayshard/internal/id"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/creack/pty"
)

// Session is a server-owned PTY. Client disconnect does not kill it.
type Session struct {
	ID        string
	ProjectID string
	File      *os.File
	Cmd       *exec.Cmd
	Created   time.Time
}

type Manager struct {
	Store *storage.Store
	mu    sync.Mutex
	live  map[string]*Session
}

func New(store *storage.Store) *Manager {
	return &Manager{Store: store, live: map[string]*Session{}}
}

func (m *Manager) Start(ctx context.Context, projectID, cwd, deviceID string) (*Session, error) {
	_ = ctx // client request context must not own the PTY lifetime
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	cmd := exec.Command(shell)
	cmd.Dir = cwd
	f, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	s := &Session{ID: id.New(), ProjectID: projectID, File: f, Cmd: cmd, Created: time.Now().UTC()}
	m.mu.Lock()
	m.live[s.ID] = s
	m.mu.Unlock()
	if m.Store != nil {
		_ = m.Store.InsertPTY(ctx, s.ID, projectID, deviceID, cwd)
	}
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		delete(m.live, s.ID)
		m.mu.Unlock()
		if m.Store != nil {
			_ = m.Store.SetPTYAlive(context.Background(), s.ID, false)
		}
	}()
	return s, nil
}

func (m *Manager) Attach(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.live[id]
	return s, ok
}

func (m *Manager) Write(id string, b []byte) error {
	s, ok := m.Attach(id)
	if !ok {
		return io.ErrClosedPipe
	}
	_, err := s.File.Write(b)
	return err
}

func (m *Manager) LostAfterRestart(ctx context.Context, id string) bool {
	m.mu.Lock()
	_, live := m.live[id]
	m.mu.Unlock()
	return !live
}

func (m *Manager) List(projectID string) []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Session
	for _, s := range m.live {
		if projectID == "" || s.ProjectID == projectID {
			out = append(out, s)
		}
	}
	return out
}

func (m *Manager) MarkLostAfterRestart(ctx context.Context) {
	if m.Store == nil {
		return
	}
	// rows still marked alive in SQLite after process start are dead
}
