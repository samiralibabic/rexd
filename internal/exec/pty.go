package exec

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/samiralibabic/rexd/internal/events"
)

type PTYSession struct {
	ID        string
	ProcessID string
	SessionID string
	Cmd       *exec.Cmd
	File      *os.File
	Cols      uint16
	Rows      uint16
	StartedAt time.Time
}

type PTYManager struct {
	mu   sync.RWMutex
	bus  *events.Bus
	ptys map[string]*PTYSession
}

func NewPTYManager(bus *events.Bus) *PTYManager {
	return &PTYManager{
		bus:  bus,
		ptys: map[string]*PTYSession{},
	}
}

func (m *PTYManager) Open(ctx context.Context, sessionID string, argv []string, shell bool, command, cwd string, env map[string]string, cols, rows uint16) (*PTYSession, error) {
	if !shell && len(argv) == 0 {
		return nil, errors.New("argv is required")
	}
	var cmd *exec.Cmd
	if shell {
		cmd = exec.CommandContext(ctx, "sh", "-lc", command)
	} else {
		cmd = exec.CommandContext(ctx, argv[0], argv[1:]...)
	}
	cmd.Dir = cwd
	if len(env) > 0 {
		for k, v := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if cols == 0 {
		cols = 120
	}
	if rows == 0 {
		rows = 32
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	ps := &PTYSession{
		ID:        fmt.Sprintf("pty_%d", time.Now().UnixNano()),
		ProcessID: fmt.Sprintf("p_%d", time.Now().UnixNano()),
		SessionID: sessionID,
		Cmd:       cmd,
		File:      ptmx,
		Cols:      cols,
		Rows:      rows,
		StartedAt: time.Now().UTC(),
	}
	m.mu.Lock()
	m.ptys[ps.ID] = ps
	m.mu.Unlock()
	go m.readOutput(ps)
	go m.wait(ps)
	return ps, nil
}

func (m *PTYManager) readOutput(ps *PTYSession) {
	scanner := bufio.NewScanner(ps.File)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	seq := 0
	for scanner.Scan() {
		seq++
		m.bus.Publish(ps.SessionID, "pty.output", map[string]any{
			"session_id": ps.SessionID,
			"pty_id":     ps.ID,
			"process_id": ps.ProcessID,
			"seq":        seq,
			"data":       scanner.Text() + "\n",
			"encoding":   "utf8",
		})
	}
}

func (m *PTYManager) wait(ps *PTYSession) {
	waitErr := ps.Cmd.Wait()
	exitCode := 0
	var sig *string
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			ws := exitErr.Sys().(syscall.WaitStatus)
			if ws.Signaled() {
				s := ws.Signal().String()
				sig = &s
			}
		}
	}
	m.bus.Publish(ps.SessionID, "pty.exit", map[string]any{
		"session_id":  ps.SessionID,
		"pty_id":      ps.ID,
		"process_id":  ps.ProcessID,
		"exit_code":   exitCode,
		"signal":      sig,
		"duration_ms": time.Since(ps.StartedAt).Milliseconds(),
	})
	_ = ps.File.Close()
	m.mu.Lock()
	delete(m.ptys, ps.ID)
	m.mu.Unlock()
}

func (m *PTYManager) Get(id string) (*PTYSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ps, ok := m.ptys[id]
	if !ok {
		return nil, errors.New("pty not found")
	}
	return ps, nil
}

func (m *PTYManager) Input(id, data string) (int, error) {
	ps, err := m.Get(id)
	if err != nil {
		return 0, err
	}
	return ps.File.Write([]byte(data))
}

func (m *PTYManager) Resize(id string, cols, rows uint16) error {
	ps, err := m.Get(id)
	if err != nil {
		return err
	}
	return pty.Setsize(ps.File, &pty.Winsize{Cols: cols, Rows: rows})
}

func (m *PTYManager) Close(id string) error {
	ps, err := m.Get(id)
	if err != nil {
		return err
	}
	_ = ps.Cmd.Process.Kill()
	return ps.File.Close()
}
