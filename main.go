package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := LoadConfig("/config.yaml")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	proxyPort := envOrDefault("PROXY_PORT", fmt.Sprintf("%d", cfg.Proxy.Port))
	mgmtPort := envOrDefault("MGMT_PORT", "7656")

	sup, err := newSupervisor(cfg, proxyPort)
	if err != nil {
		log.Fatalf("supervisor: %v", err)
	}

	log.Println("startup: waiting for POST /token to activate the proxy")

	// Proxy server — forwards requests to upstream with injected token.
	proxySrv := &http.Server{
		Addr:    ":" + proxyPort,
		Handler: sup,
	}

	// Management API server.
	mgmtSrv := &http.Server{
		Addr:    ":" + mgmtPort,
		Handler: newAPI(sup),
	}

	go func() {
		log.Printf("proxy listening on :%s → %s", proxyPort, cfg.Proxy.UpstreamURL)
		if err := proxySrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("proxy server: %v", err)
		}
	}()

	go func() {
		log.Printf("management API listening on :%s", mgmtPort)
		if err := mgmtSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("api: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	proxySrv.Shutdown(ctx) //nolint:errcheck
	mgmtSrv.Shutdown(ctx)  //nolint:errcheck
	sup.stop()
	log.Println("done")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
