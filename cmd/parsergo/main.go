package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"parsergo/internal/api"
	"parsergo/internal/job"
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

	// Create job store
	jobStore := job.NewStore()

	// Create API handler
	analysisHandler := api.NewHandler(api.HandlerConfig{
		Logger:       logger,
		JobStore:     jobStore,
		MaxInputSize: 10 * 1024 * 1024, // 10MB
	})

	// Create report handler
	reportHandler := api.NewReportHandler(analysisHandler, logger)

	// Register routes
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Set ready state after initialization (VAL-SVC-002)
	analysisHandler.SetReady(true)
	logger.Info("service ready")

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
	}()

	logger.Info("starting server", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}
