package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"minator/api"
	"minator/monitor"
	"minator/repository"
)

func main() {
	// Structured shutdown with signal handling
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize resources
	db, err := repository.InitDb()
	if err != nil {
		slog.Error("Failed to init DB", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Warn("Failed to Close DB", "error", err)
		}
	}()

	ss := repository.NewServiceStatusRepo(db)
	hm := repository.NewHardwareMetricsRepo(db)
	h := api.NewHandler(ss, hm)

	// Set up HTTP server
	fs := http.FileServer(http.Dir("templates/static"))
	mux := http.NewServeMux()
	mux.Handle("/templates/static/", http.StripPrefix("/templates/static", fs))
	mux.HandleFunc("GET /status", h.StatusPageHandler)
	mux.HandleFunc("POST /api/service/status", h.ServiceStatusHandler())
	mux.HandleFunc("GET /api/stream/hardware-metrics", h.StreamHardwareMetrics(ctx))
	mux.HandleFunc("GET /api/stream/service-statuses", h.StreamServiceStatuses(ctx))

	port := getPort()
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start periodic health checks
	monitor := monitor.NewMonitor()
	monitorCtx, monitorCancel := context.WithCancel(ctx)
	defer monitorCancel()
	go monitor.Run(monitorCtx)

	// Start HTTP server in a goroutine
	go func() {
		slog.Info("Server is starting", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	slog.Info("Shutdown signal received, initiating graceful shutdown...")

	// Create a context with timeout for shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown the HTTP server gracefully
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server shutdown failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Server gracefully stopped")
}

func getPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "18080"
}
