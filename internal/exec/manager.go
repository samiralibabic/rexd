package exec

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/samiralibabic/rexd/internal/events"
)

type ProcessState struct {
	Status      string
	ExitCode    *int
	Signal      *string
	BytesStdout int64
	BytesStderr int64
	TimedOut    bool
}

type RunningProcess struct {
	ID            string
	SessionID     string
	Cmd           *exec.Cmd
	Stdin         io.WriteCloser
	StartedAt     time.Time
	MaxOutput     int64
	BytesStdout   int64
	BytesStderr   int64
	StdoutSeq     int64
	StderrSeq     int64
	Detached      bool
	TimedOut      bool
	ExitCh        chan ProcessState
	cancelTimeout context.CancelFunc
	mu            sync.Mutex
}

func (p *RunningProcess) CancelTimeout(cancel context.CancelFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelTimeout = cancel
}

func (p *RunningProcess) MarkTimedOut() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.TimedOut = true
}

type Manager struct {
	mu        sync.RWMutex
	processes map[string]*RunningProcess
	bus       *events.Bus
}

func NewManager(bus *events.Bus) *Manager {
	return &Manager{
		processes: map[string]*RunningProcess{},
		bus:       bus,
	}
}

func (m *Manager) Add(p *RunningProcess) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processes[p.ID] = p
}

func (m *Manager) Get(id string) (*RunningProcess, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.processes[id]
	if !ok {
		return nil, errors.New("process not found")
	}
	return p, nil
}

func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.processes, id)
}

func (m *Manager) Kill(id, sig string) error {
	p, err := m.Get(id)
	if err != nil {
		return err
	}
	s := syscall.SIGTERM
	if sig == "KILL" {
		s = syscall.SIGKILL
	}
	return p.Cmd.Process.Signal(s)
}

func (m *Manager) WireStreams(p *RunningProcess, stdout, stderr io.Reader) {
	go m.pipeStream(p, stdout, "exec.stdout")
	go m.pipeStream(p, stderr, "exec.stderr")
}

func (m *Manager) pipeStream(p *RunningProcess, r io.Reader, method string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text() + "\n"
		p.mu.Lock()
		if method == "exec.stdout" {
			p.StdoutSeq++
			p.BytesStdout += int64(len(line))
		} else {
			p.StderrSeq++
			p.BytesStderr += int64(len(line))
		}
		seq := p.StdoutSeq
		if method == "exec.stderr" {
			seq = p.StderrSeq
		}
		total := p.BytesStdout + p.BytesStderr
		maxOutput := p.MaxOutput
		p.mu.Unlock()
		if maxOutput > 0 && total > maxOutput {
			_ = p.Cmd.Process.Kill()
			p.mu.Lock()
			p.TimedOut = true
			p.mu.Unlock()
			return
		}
		m.bus.Publish(p.SessionID, method, map[string]any{
			"session_id": p.SessionID,
			"process_id": p.ID,
			"seq":        seq,
			"data":       line,
			"encoding":   "utf8",
		})
	}
}

func (m *Manager) Wait(p *RunningProcess, waitErr error) ProcessState {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := ProcessState{
		Status:      "exited",
		BytesStdout: p.BytesStdout,
		BytesStderr: p.BytesStderr,
		TimedOut:    p.TimedOut,
	}
	if p.cancelTimeout != nil {
		p.cancelTimeout()
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			code := exitErr.ExitCode()
			state.ExitCode = &code
			ws := exitErr.Sys().(syscall.WaitStatus)
			if ws.Signaled() {
				s := ws.Signal().String()
				state.Signal = &s
				state.Status = "killed"
			}
		}
	} else {
		code := 0
		state.ExitCode = &code
	}
	return state
}
