package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/samiralibabic/rexd/internal/config"
	"github.com/samiralibabic/rexd/internal/protocol"
	"github.com/samiralibabic/rexd/internal/server"
)

func TestStdioSessionAndFSFlow(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Default()
	cfg.Security.AllowedRoot = []config.AllowedRoot{{Path: tmp}}
	svc, err := server.NewService(cfg)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	client, srv := net.Pipe()
	defer client.Close()
	go func() {
		_ = server.RunStdio(context.Background(), svc, srv, srv)
	}()

	enc := json.NewEncoder(client)
	dec := bufio.NewReader(client)

	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "session.open",
		"params": map[string]any{
			"client_name":     "test-client",
			"workspace_roots": []string{tmp},
		},
	}); err != nil {
		t.Fatalf("encode session.open: %v", err)
	}
	var line map[string]any
	if err := readLine(dec, &line); err != nil {
		t.Fatalf("read session.open response: %v", err)
	}
	result := line["result"].(map[string]any)
	sessionID := result["session_id"].(string)

	target := filepath.Join(tmp, "hello.txt")
	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "fs.write",
		"params": map[string]any{
			"session_id":    sessionID,
			"path":          target,
			"content":       "hello\n",
			"encoding":      "utf8",
			"mode":          "replace",
			"mkdir_parents": true,
		},
	}); err != nil {
		t.Fatalf("encode fs.write: %v", err)
	}
	if err := readLine(dec, &line); err != nil {
		t.Fatalf("read fs.write response: %v", err)
	}

	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "fs.read",
		"params": map[string]any{
			"session_id": sessionID,
			"path":       target,
		},
	}); err != nil {
		t.Fatalf("encode fs.read: %v", err)
	}
	if err := readLine(dec, &line); err != nil {
		t.Fatalf("read fs.read response: %v", err)
	}
	readRes := line["result"].(map[string]any)
	if got := readRes["content"].(string); got != "hello\n" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestStdioExecStartAndExitEvent(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Default()
	cfg.Security.AllowedRoot = []config.AllowedRoot{{Path: tmp}}
	cfg.Limits.DefaultTimeoutMs = 1500
	svc, err := server.NewService(cfg)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	client, srv := net.Pipe()
	defer client.Close()
	go func() {
		_ = server.RunStdio(context.Background(), svc, srv, srv)
	}()

	enc := json.NewEncoder(client)
	dec := bufio.NewReader(client)

	_ = enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "session.open",
		"params": map[string]any{"client_name": "test", "workspace_roots": []string{tmp}},
	})
	var msg map[string]any
	_ = readLine(dec, &msg)
	sessionID := msg["result"].(map[string]any)["session_id"].(string)

	_ = enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "exec.start",
		"params": map[string]any{
			"session_id": sessionID,
			"argv":       []string{"sh", "-lc", "printf test-output"},
			"cwd":        tmp,
		},
	})
	_ = readLine(dec, &msg)

	timeout := time.After(3 * time.Second)
	gotExit := false
	for !gotExit {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for exec.exit")
		default:
			var notif protocol.Notification
			if err := readLine(dec, &notif); err != nil {
				t.Fatalf("read event: %v", err)
			}
			if notif.Method == "exec.exit" {
				gotExit = true
			}
		}
	}
}

func readLine(reader *bufio.Reader, out any) error {
	raw, err := reader.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
