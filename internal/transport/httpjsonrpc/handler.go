package httpjsonrpc

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/samiralibabic/rexd/internal/protocol"
)

type RequestHandler func(context.Context, protocol.Request) protocol.Response

func Handler(handle RequestHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req protocol.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		resp := handle(r.Context(), req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
