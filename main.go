package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"minator/api"
	"minator/monitor"
)

func main() {
	// Structured shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start periodic health checks
	monitor := monitor.NewMonitor()
	go monitor.Run(context.Background())

	// Register HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", api.StatusPageHandler)
	mux.HandleFunc("POST /api/service/status", api.ServiceStatusHandler(monitor))
	mux.HandleFunc("/api/hardware/metrics/stream", api.StreamHardwareMetrics(monitor))
	mux.HandleFunc("/events", api.EventsHandler(monitor))

	port := getPort()
	slog.Info("Server is starting", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		panic(err)
	}

	<-ctx.Done()
	slog.Info("Shutdown signal received. Exiting...")
}

func getPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "18080"
}
