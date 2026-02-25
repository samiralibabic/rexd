package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/samiralibabic/rexd/internal/config"
	"github.com/samiralibabic/rexd/internal/server"
	"github.com/samiralibabic/rexd/internal/transport/httpjsonrpc"
	"github.com/samiralibabic/rexd/internal/transport/wsjsonrpc"
)

func TestHTTPJSONRPCSessionOpen(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Default()
	cfg.Security.AllowedRoot = []config.AllowedRoot{{Path: tmp}}
	svc, err := server.NewService(cfg)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", httpjsonrpc.Handler(svc.Handle))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "session.open",
		"params": map[string]any{
			"client_name":     "http-test",
			"workspace_roots": []string{tmp},
		},
	}
	raw, _ := json.Marshal(payload)
	resp, err := http.Post(ts.URL+"/rpc", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("http post: %v", err)
	}
	defer resp.Body.Close()
	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded["error"] != nil {
		t.Fatalf("unexpected error response: %+v", decoded["error"])
	}
}

func TestWebSocketJSONRPCRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Default()
	cfg.Security.AllowedRoot = []config.AllowedRoot{{Path: tmp}}
	svc, err := server.NewService(cfg)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsjsonrpc.Handler(svc.Handle, svc.Bus().Subscribe))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + ts.URL[len("http"):] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "session.open",
		"params": map[string]any{
			"client_name":     "ws-test",
			"workspace_roots": []string{tmp},
		},
	}
	if err := conn.WriteJSON(req); err != nil {
		t.Fatalf("write ws req: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg map[string]any
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read ws response: %v", err)
	}
	if msg["error"] != nil {
		t.Fatalf("unexpected ws error: %+v", msg["error"])
	}
}
