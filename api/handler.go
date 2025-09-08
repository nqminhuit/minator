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

func StreamHardwareMetrics(m *monitor.Monitor) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		lastTimestamp := sendInitialMetrics(m, w, flusher, 50)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				slog.Info("Client disconnected from hardware metrics SSE")
				return

			case <-ticker.C:
				newTimestamp := sendNewMetrics(m, w, flusher, lastTimestamp)
				if !newTimestamp.IsZero() {
					lastTimestamp = newTimestamp
				}
			}
		}
	}
}

func sendInitialMetrics(m *monitor.Monitor, w http.ResponseWriter, f http.Flusher, limit int) time.Time {
	query := `
		SELECT timestamp, cpu_percent, ram_percent, disk_percent
		FROM (
			SELECT timestamp, cpu_percent, ram_percent, disk_percent
			FROM hardware_metrics
			WHERE timestamp >= NOW() - INTERVAL '24 hours'
			ORDER BY timestamp DESC
			LIMIT $1
		) sub
		ORDER BY timestamp ASC;`

	rows, err := m.DB.Query(query, limit)
	if err != nil {
		slog.Error("Failed to query initial hardware metrics", "error", err)
		http.Error(w, "Failed to query hardware metrics", http.StatusInternalServerError)
		return time.Time{}
	}
	defer rows.Close()

	var lastTimestamp time.Time
	for rows.Next() {
		var m data.HardwareMetrics
		if err := rows.Scan(&m.Timestamp, &m.CPUPercent, &m.RAMPercent, &m.DiskPercent); err != nil {
			slog.Error("Failed to scan hardware metrics", "error", err)
			continue
		}
		lastTimestamp = m.Timestamp

		if jsonData, err := json.Marshal(m); err == nil {
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			f.Flush()
		}
	}

	if err := rows.Err(); err != nil {
		slog.Error("Error iterating initial rows", "error", err)
	}

	return lastTimestamp
}

// sendNewMetrics only fetches rows newer than lastTimestamp
func sendNewMetrics(m *monitor.Monitor, w http.ResponseWriter, f http.Flusher, lastTimestamp time.Time) time.Time {
	query := `
		SELECT timestamp, cpu_percent, ram_percent, disk_percent
		FROM hardware_metrics
		WHERE timestamp > $1
		ORDER BY timestamp ASC
	`

	rows, err := m.DB.Query(query, lastTimestamp)
	if err != nil {
		slog.Error("Failed to query new hardware metrics", "error", err)
		return time.Time{}
	}
	defer rows.Close()

	var newestTimestamp time.Time
	for rows.Next() {
		var m data.HardwareMetrics
		if err := rows.Scan(&m.Timestamp, &m.CPUPercent, &m.RAMPercent, &m.DiskPercent); err != nil {
			slog.Error("Failed to scan hardware metrics", "error", err)
			continue
		}

		newestTimestamp = m.Timestamp

		if jsonData, err := json.Marshal(m); err == nil {
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			f.Flush()
		}
	}

	if err := rows.Err(); err != nil {
		slog.Error("Error iterating new rows", "error", err)
	}

	return newestTimestamp
}
