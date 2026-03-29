package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sagaragas/parser-go/internal/api"
	"github.com/sagaragas/parser-go/internal/job"
)

const (
	defaultStartupReadinessDelay = 250 * time.Millisecond
	startupProbeInterval         = 10 * time.Millisecond
	startupProbeTimeout          = 5 * time.Second
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintf(os.Stderr, "Usage: %s serve\n", os.Args[0])
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	addr := os.Getenv("PARSERGO_ADDR")
	if addr == "" {
		addr = "127.0.0.1:3120"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("listen error", "error", err)
		os.Exit(1)
	}

	if err := serveWithListener(ctx, logger, listener, defaultStartupReadinessDelay); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func serveWithListener(ctx context.Context, logger *slog.Logger, listener net.Listener, startupDelay time.Duration) error {
	addr := listener.Addr().String()

	jobStore := job.NewStore()
	analysisHandler := api.NewHandler(api.HandlerConfig{
		Logger:       logger,
		JobStore:     jobStore,
		MaxInputSize: envInt64("PARSERGO_MAX_UPLOAD_BYTES", 10*1024*1024, logger),
		QueueLimit:   envInt("PARSERGO_QUEUE_LIMIT", 2, logger),
		Retention:    envDuration("PARSERGO_RETENTION", 24*time.Hour, logger),
	})
	reportHandler := api.NewReportHandler(analysisHandler, logger)

	mux := http.NewServeMux()
	analysisHandler.RegisterRoutes(mux)
	reportHandler.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
	}()

	go markReadyOnceLive(ctx, logger, analysisHandler, addr, startupDelay)

	logger.Info("starting server", "addr", addr)
	err := srv.Serve(listener)
	logger.Info("server stopped")
	return err
}

func markReadyOnceLive(ctx context.Context, logger *slog.Logger, analysisHandler *api.Handler, addr string, startupDelay time.Duration) {
	if !waitForHealthz(ctx, addr) {
		logger.Warn("healthz never became reachable; leaving service unready", "addr", addr)
		return
	}

	if startupDelay > 0 {
		timer := time.NewTimer(startupDelay)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}

	analysisHandler.SetReady(true)
	logger.Info("service ready")
}

func waitForHealthz(ctx context.Context, addr string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, startupProbeTimeout)
	defer cancel()

	client := &http.Client{Timeout: 100 * time.Millisecond}
	url := "http://" + addr + "/healthz"

	for {
		req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return true
				}
			}
		}

		select {
		case <-probeCtx.Done():
			return false
		case <-time.After(startupProbeInterval):
		}
	}
}

func envInt(name string, fallback int, logger *slog.Logger) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		logger.Warn("invalid integer environment override; using fallback", "name", name, "value", value, "fallback", fallback)
		return fallback
	}
	return parsed
}

func envInt64(name string, fallback int64, logger *slog.Logger) int64 {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		logger.Warn("invalid integer environment override; using fallback", "name", name, "value", value, "fallback", fallback)
		return fallback
	}
	return parsed
}

func envDuration(name string, fallback time.Duration, logger *slog.Logger) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		logger.Warn("invalid duration environment override; using fallback", "name", name, "value", value, "fallback", fallback.String())
		return fallback
	}
	return parsed
}
