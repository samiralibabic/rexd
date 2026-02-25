package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/samiralibabic/rexd/internal/config"
	"github.com/samiralibabic/rexd/internal/server"
)

func main() {
	var cfgPath string
	var stdio bool
	var httpListen string
	flag.StringVar(&cfgPath, "config", "/etc/rexd/config.toml", "path to rexd config")
	flag.BoolVar(&stdio, "stdio", false, "run JSON-RPC on stdio")
	flag.StringVar(&httpListen, "http", "", "listen address for HTTP/WS transport")
	flag.Parse()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if stdio {
		cfg.Server.Stdio = true
	}
	if httpListen != "" {
		cfg.Server.HTTPListen = httpListen
	}

	svc, err := server.NewService(cfg)
	if err != nil {
		log.Fatalf("create service: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if cfg.Server.Stdio {
		if err := server.RunStdio(ctx, svc, os.Stdin, os.Stdout); err != nil {
			log.Fatalf("stdio server failed: %v", err)
		}
		return
	}

	if cfg.Server.HTTPListen == "" {
		log.Fatal("either --stdio or --http must be configured")
	}
	if err := server.RunHTTP(ctx, cfg, svc); err != nil {
		log.Fatalf("http server failed: %v", err)
	}
}
