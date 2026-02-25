package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var ErrNotFound = errors.New("session not found")

type Session struct {
	ID             string
	ClientName     string
	ClientVersion  string
	WorkspaceRoots []string
	CWD            string
	CreatedAt      time.Time
	ProcessCount   int
}

type Manager struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	maxActive int
}

func NewManager(maxActive int) *Manager {
	return &Manager{
		sessions:  map[string]*Session{},
		maxActive: maxActive,
	}
}

func (m *Manager) Open(clientName, clientVersion string, roots []string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.maxActive > 0 && len(m.sessions) >= m.maxActive {
		return nil, errors.New("max concurrent sessions reached")
	}
	id := "s_" + randHex(6)
	cwd := "/"
	if len(roots) > 0 {
		cwd = roots[0]
	}
	s := &Session{
		ID:             id,
		ClientName:     clientName,
		ClientVersion:  clientVersion,
		WorkspaceRoots: roots,
		CWD:            cwd,
		CreatedAt:      time.Now().UTC(),
	}
	m.sessions[id] = s
	return s, nil
}

func (m *Manager) Get(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

func (m *Manager) Close(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return ErrNotFound
	}
	delete(m.sessions, id)
	return nil
}

func (m *Manager) SetCWD(id, cwd string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	s.CWD = cwd
	return nil
}

func (m *Manager) IncProcess(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	s.ProcessCount++
	return nil
}

func (m *Manager) DecProcess(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	if s.ProcessCount > 0 {
		s.ProcessCount--
	}
	return nil
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
