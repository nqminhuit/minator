package main

import (
	"log"
	"net/http"
	"os"

	"minator/api"
	"minator/monitor"
)

func main() {
	log.Println("[healthmon] starting...")

	// Start periodic health checks
	go monitor.StartMonitorLoop()

	// Register HTTP routes
	http.HandleFunc("/status", api.StatusPageHandler)
	http.HandleFunc("/api/status", api.JSONStatusHandler)
	http.HandleFunc("/health/backup", api.BackupPingHandler)

	// Start server
	port := getPort()
	log.Printf("[healthmon] listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func getPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8080"
}
