package main

import (
	"log/slog"
	"net/http"
	"os"

	"minator/api"
	"minator/monitor"
)

func main() {
	// Start periodic health checks
	go monitor.StartMonitorLoop()

	// Register HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", api.StatusPageHandler)
	mux.HandleFunc("POST /api/service/status", api.ServiceStatusHandler)
	mux.HandleFunc("/events", api.EventsHandler)

	port := getPort()
	slog.Info("Server is starting", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		panic(err)
	}
}

func getPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "18080"
}
