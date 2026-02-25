package server

import (
	"context"
	"net/http"

	"github.com/samiralibabic/rexd/internal/config"
	"github.com/samiralibabic/rexd/internal/transport/httpjsonrpc"
	"github.com/samiralibabic/rexd/internal/transport/wsjsonrpc"
)

func RunHTTP(ctx context.Context, cfg config.Config, svc *Service) error {
	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Server.HTTPPath, httpjsonrpc.Handler(svc.Handle))
	mux.HandleFunc(cfg.Server.WSPath, wsjsonrpc.Handler(svc.Handle, svc.Bus().Subscribe))
	srv := &http.Server{
		Addr:    cfg.Server.HTTPListen,
		Handler: mux,
	}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	return srv.ListenAndServe()
}
