package audit

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Entry struct {
	Timestamp string `json:"timestamp"`
	SessionID string `json:"session_id,omitempty"`
	Client    string `json:"client_name,omitempty"`
	Method    string `json:"method"`
	Params    any    `json:"params,omitempty"`
	Result    any    `json:"result,omitempty"`
	Error     any    `json:"error,omitempty"`
}

type Logger struct {
	enabled bool
	path    string
	mu      sync.Mutex
}

func New(enabled bool, path string) *Logger {
	return &Logger{enabled: enabled, path: path}
}

func (l *Logger) Write(entry Entry) {
	if !l.enabled || l.path == "" {
		return
	}
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	raw, err := json.Marshal(entry)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(raw)
	_, _ = f.WriteString("\n")
}
