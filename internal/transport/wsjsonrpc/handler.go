package wsjsonrpc

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/samiralibabic/rexd/internal/protocol"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type RequestHandler func(context.Context, protocol.Request) protocol.Response
type SubscribeFunc func(string) (chan protocol.Notification, func())

func Handler(handle RequestHandler, subscribe SubscribeFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		subscriptions := map[string]func(){}
		defer func() {
			for _, unsub := range subscriptions {
				unsub()
			}
		}()

		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req protocol.Request
			if err := json.Unmarshal(payload, &req); err != nil {
				return
			}
			resp := handle(r.Context(), req)
			raw, _ := json.Marshal(resp)
			if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
				return
			}
			var sid struct {
				SessionID string `json:"session_id"`
			}
			_ = json.Unmarshal(req.Params, &sid)
			if sid.SessionID != "" {
				if _, ok := subscriptions[sid.SessionID]; !ok {
					ch, unsub := subscribe(sid.SessionID)
					subscriptions[sid.SessionID] = unsub
					go func() {
						for evt := range ch {
							evtRaw, _ := json.Marshal(evt)
							_ = conn.WriteMessage(websocket.TextMessage, evtRaw)
						}
					}()
				}
			}
		}
	}
}
