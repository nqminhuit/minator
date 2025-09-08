package api

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"minator/data"
	"minator/monitor"
	"net/http"
	"time"
)

func StatusPageHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/status.html"))
	if err := tmpl.Execute(w, nil); err != nil {
		http.Error(w, "Render error", 500)
	}
}

// While backup is in progress, it will send a curl command to this server,
// we will store health status like which backup is done, which is in progress,
// which fails, ... Then this function will update response on a json file
// so that StatusPageHandler will use this json to render the html
func ServiceStatusHandler(m *monitor.Monitor) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload data.ServiceRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			slog.Error("Could not decode ServiceRequest", "err", err)
			return
		}
		statuses := make([]data.ServiceStatus, 1)
		statuses = append(statuses, payload.ToHealthStatus(time.Now()))
		m.InsertServiceStatus(statuses)
	}
}

func sendStatuses(m *monitor.Monitor, flusher http.Flusher, w http.ResponseWriter) {
	// Read status from JSON file
	content, err := m.GetLatestServiceStatus()
	if err != nil {
		slog.Error("Failed to get health status", "err", err)
		fmt.Fprintf(w, "event: error\ndata: %v\n\n", err)
		flusher.Flush()
		return
	}
	if len(content) < 1 {
		return
	}
	json, err := data.ServiceStatusToJSON(content)
	if err != nil {
		slog.Error("Failed to parse health status to json", "err", err)
		fmt.Fprintf(w, "event: error\ndata: %v\n\n", err)
		flusher.Flush()
		return
	}
	// Write event to stream
	fmt.Fprintf(w, "event: update\ndata: %s\n\n", json)
	flusher.Flush()
}

func EventsHandler(m *monitor.Monitor) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("SSE client connected", "remote", r.RemoteAddr)
		defer slog.Info("SSE client disconnected", "remote", r.RemoteAddr)

		// SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // this header ensures the stream isn't buffered if served behind NGINX

		// Allow CORS (only needed if frontend is on a different origin)
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Flush writer immediately
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		sendStatuses(m, flusher, w)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return // client disconnected
			case <-ticker.C:
				sendStatuses(m, flusher, w)
			}
		}
	}
}

func GetHardwareMetrics(m *monitor.Monitor) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		query := `
		SELECT timestamp, cpu_percent, ram_percent, disk_percent
		FROM hardware_metrics
		WHERE timestamp >= NOW() - INTERVAL '24 hours'
		ORDER BY timestamp`

		rows, err := m.DB.Query(query)
		if err != nil {
			slog.Error("Failed to query hardware metrics", "error", err)
			http.Error(w, "Failed to query hardware metrics", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var metrics []data.HardwareMetrics
		for rows.Next() {
			var m data.HardwareMetrics
			if err := rows.Scan(&m.Timestamp, &m.CPUPercent, &m.RAMPercent, &m.DiskPercent); err != nil {
				slog.Error("Failed to scan hardware metrics", "error", err)
				http.Error(w, "Failed to scan hardware metrics", http.StatusInternalServerError)
				return
			}
			metrics = append(metrics, m)
		}

		if err := rows.Err(); err != nil {
			slog.Error("Error iterating over rows", "error", err)
			http.Error(w, "Failed to scan hardware metrics", http.StatusInternalServerError)
			return
		}

		// Set Content-Type to application/json
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(metrics); err != nil {
			slog.Error("Failed to encode hardware metrics", "error", err)
			http.Error(w, fmt.Sprintf("Failed to encode hardware metrics: %v", err), http.StatusInternalServerError)
		}
	}
}
