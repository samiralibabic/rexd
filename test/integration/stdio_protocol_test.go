package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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

func TestStdioExecStartShellDefaultsToNonLogin(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	profilePath := filepath.Join(home, ".profile")
	if err := os.WriteFile(profilePath, []byte("export REXD_LOGIN_MARKER=from_profile\n"), 0644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

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
	sessionID := openSession(t, enc, dec, tmp)

	stdout := runExecAndCollectStdout(t, enc, dec, map[string]any{
		"session_id": sessionID,
		"shell":      true,
		"command":    "printf %s \"${REXD_LOGIN_MARKER:-missing}\"",
		"cwd":        tmp,
		"env": map[string]any{
			"HOME": home,
		},
	})

	if stdout != "missing" {
		t.Fatalf("expected non-login shell to skip profile marker, got %q", stdout)
	}
}

func TestStdioExecStartShellLoginCompatibility(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	profilePath := filepath.Join(home, ".profile")
	if err := os.WriteFile(profilePath, []byte("export REXD_LOGIN_MARKER=from_profile\n"), 0644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

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
	sessionID := openSession(t, enc, dec, tmp)

	stdout := runExecAndCollectStdout(t, enc, dec, map[string]any{
		"session_id": sessionID,
		"shell":      true,
		"login":      true,
		"command":    "printf %s \"${REXD_LOGIN_MARKER:-missing}\"",
		"cwd":        tmp,
		"env": map[string]any{
			"HOME": home,
		},
	})

	if stdout != "from_profile" {
		t.Fatalf("expected login shell to load profile marker, got %q", stdout)
	}
}

func TestStdioFSEditAndPatchFlow(t *testing.T) {
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
	sessionID := line["result"].(map[string]any)["session_id"].(string)

	target := filepath.Join(tmp, "edit-target.txt")
	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "fs.write",
		"params": map[string]any{
			"session_id":    sessionID,
			"path":          target,
			"content":       "hello\nworld\n",
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
		"method":  "fs.edit",
		"params": map[string]any{
			"session_id": sessionID,
			"path":       target,
			"old_string": "world",
			"new_string": "earth",
		},
	}); err != nil {
		t.Fatalf("encode fs.edit: %v", err)
	}
	if err := readLine(dec, &line); err != nil {
		t.Fatalf("read fs.edit response: %v", err)
	}

	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
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
	if got := line["result"].(map[string]any)["content"].(string); got != "hello\nearth\n" {
		t.Fatalf("unexpected edit content: %q", got)
	}

	deletePath := filepath.Join(tmp, "delete-me.txt")
	if err := os.WriteFile(deletePath, []byte("remove me\n"), 0644); err != nil {
		t.Fatalf("seed delete file: %v", err)
	}
	addedPath := filepath.Join(tmp, "added-from-patch.txt")
	patchText := fmt.Sprintf("*** Begin Patch\n*** Update File: %s\n@@\n-earth\n+patched\n*** Add File: %s\n+created from patch\n*** Delete File: %s\n*** End Patch", target, addedPath, deletePath)

	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "fs.patch",
		"params": map[string]any{
			"session_id": sessionID,
			"patch_text": patchText,
		},
	}); err != nil {
		t.Fatalf("encode fs.patch: %v", err)
	}
	if err := readLine(dec, &line); err != nil {
		t.Fatalf("read fs.patch response: %v", err)
	}
	if line["error"] != nil {
		t.Fatalf("fs.patch returned error: %+v", line["error"])
	}

	updatedContent, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read updated target: %v", err)
	}
	if string(updatedContent) != "hello\npatched\n" {
		t.Fatalf("unexpected patched content: %q", string(updatedContent))
	}

	if _, err := os.Stat(addedPath); err != nil {
		t.Fatalf("expected added file to exist: %v", err)
	}
	if _, err := os.Stat(deletePath); !os.IsNotExist(err) {
		t.Fatalf("expected delete target to be removed, got err=%v", err)
	}
}

func readLine(reader *bufio.Reader, out any) error {
	raw, err := reader.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func openSession(t *testing.T, enc *json.Encoder, dec *bufio.Reader, workspaceRoot string) string {
	t.Helper()
	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "session.open",
		"params": map[string]any{
			"client_name":     "test-client",
			"workspace_roots": []string{workspaceRoot},
		},
	}); err != nil {
		t.Fatalf("encode session.open: %v", err)
	}
	var line map[string]any
	if err := readLine(dec, &line); err != nil {
		t.Fatalf("read session.open response: %v", err)
	}
	result := line["result"].(map[string]any)
	return result["session_id"].(string)
}

func runExecAndCollectStdout(t *testing.T, enc *json.Encoder, dec *bufio.Reader, params map[string]any) string {
	t.Helper()
	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "exec.start",
		"params":  params,
	}); err != nil {
		t.Fatalf("encode exec.start: %v", err)
	}

	var line map[string]any
	if err := readLine(dec, &line); err != nil {
		t.Fatalf("read exec.start response: %v", err)
	}
	if line["error"] != nil {
		t.Fatalf("exec.start returned error: %+v", line["error"])
	}

	var stdout strings.Builder
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for exec.exit")
		default:
			var notif protocol.Notification
			if err := readLine(dec, &notif); err != nil {
				t.Fatalf("read event: %v", err)
			}
			if notif.Method == "exec.stdout" {
				if paramsMap, ok := notif.Params.(map[string]any); ok {
					if data, ok := paramsMap["data"].(string); ok {
						stdout.WriteString(data)
					}
				}
			}
			if notif.Method == "exec.exit" {
				return strings.TrimSpace(stdout.String())
			}
		}
	}
}
