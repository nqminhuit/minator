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
	"minator/repository"
)

func main() {
	// Structured shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fs := http.FileServer(http.Dir("templates/static"))

	// Start periodic health checks
	monitor := monitor.NewMonitor()
	go monitor.Run(context.Background())

	db := repository.InitDb()
	ss := repository.NewServiceStatusRepo(db)
	hm := repository.NewHardwareMetricsRepo(db)

	h := api.NewHandler(ss, hm)

	// Register HTTP routes
	mux := http.NewServeMux()
	mux.Handle("/templates/static/", http.StripPrefix("/templates/static", fs))
	mux.HandleFunc("GET /status", h.StatusPageHandler)
	mux.HandleFunc("POST /api/service/status", h.ServiceStatusHandler())
	mux.HandleFunc("/api/hardware/metrics/stream", h.StreamHardwareMetrics())
	mux.HandleFunc("/events", h.EventsHandler())

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
