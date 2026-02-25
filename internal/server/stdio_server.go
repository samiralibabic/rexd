package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/samiralibabic/rexd/internal/protocol"
	"github.com/samiralibabic/rexd/internal/transport/ndjson"
)

func RunStdio(ctx context.Context, svc *Service, in io.Reader, out io.Writer) error {
	dec := ndjson.NewDecoder(in)
	enc := ndjson.NewEncoder(out)
	activeSessionSubs := map[string]func(){}

	for {
		var req protocol.Request
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if req.ID != nil {
			resp := svc.Handle(ctx, req)
			if err := enc.Encode(resp); err != nil {
				return err
			}
		} else {
			_ = svc.Handle(ctx, req)
		}

		var sid struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal(req.Params, &sid)
		if sid.SessionID != "" {
			if _, ok := activeSessionSubs[sid.SessionID]; !ok {
				ch, unsub := svc.Bus().Subscribe(sid.SessionID)
				activeSessionSubs[sid.SessionID] = unsub
				go func() {
					for evt := range ch {
						_ = enc.Encode(evt)
					}
				}()
			}
		}
	}
}
