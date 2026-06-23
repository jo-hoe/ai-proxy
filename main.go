package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	levelStr := envOrDefault("LOG_LEVEL", "INFO")
	var level slog.Level
	if err := level.UnmarshalText([]byte(levelStr)); err != nil {
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	cfg, err := LoadConfig("/config.yaml")
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	proxyPort := envOrDefault("PROXY_PORT", fmt.Sprintf("%d", cfg.Proxy.Port))
	mgmtPort := envOrDefault("MGMT_PORT", "7656")

	sup, err := newSupervisor(cfg, proxyPort)
	if err != nil {
		slog.Error("supervisor init", "err", err)
		os.Exit(1)
	}

	slog.Info("startup: waiting for POST /token to activate the proxy")

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
		slog.Info("proxy listening", "port", proxyPort, "upstream", cfg.Proxy.UpstreamURL)
		if err := proxySrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("proxy server", "err", err)
			os.Exit(1)
		}
	}()

	go func() {
		slog.Info("management API listening", "port", mgmtPort)
		if err := mgmtSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("api server", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	slog.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	proxySrv.Shutdown(ctx) //nolint:errcheck
	mgmtSrv.Shutdown(ctx)  //nolint:errcheck
	sup.stop()
	slog.Info("done")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
