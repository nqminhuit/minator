package api

import (
	"database/sql"
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

func StreamHardwareMetrics(m *monitor.Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		group := r.URL.Query().Get("group")
		if group == "" {
			group = "none"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		var lastTimestamp time.Time
		lastTimestamp = getMetrics(m, w, flusher, group, lastTimestamp)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				slog.Info("Client disconnected from hardware metrics SSE")
				return
			case <-ticker.C:
				lastTimestamp = getMetrics(m, w, flusher, group, lastTimestamp)
			}
		}
	}
}

// limit based on group
func calculateLimit(group string) int {
	switch group {
	case "minute":
		return 200 // 100 minute samples
	case "hour":
		return 200 * 60 // 100 hour samples
	case "day":
		return 200 * 60 * 24 // 100 day samples
	case "month":
		return 200 * 60 * 24 * 30 // 300 day samples
	default:
		return 100 // 100 second samples
	}
}

// getMetrics sends metrics (raw or grouped).
// - limit is used for the initial batch when lastTimestamp.IsZero().
// - group=="none" => raw rows; otherwise group is date_trunc unit.
func getMetrics(mon *monitor.Monitor, w http.ResponseWriter, f http.Flusher, group string, lastTimestamp time.Time) time.Time {
	var (
		rows *sql.Rows
		err  error
	)

	limit := calculateLimit(group)

	switch group {
	case "none":
		if lastTimestamp.IsZero() {
			// Initial raw batch: get latest N rows, then send in chronological order
			query := `
				SELECT timestamp, cpu_percent, ram_percent, disk_percent
				FROM (
					SELECT timestamp, cpu_percent, ram_percent, disk_percent
					FROM hardware_metrics
					ORDER BY timestamp DESC
					LIMIT $1
				) sub
				ORDER BY timestamp ASC;
			`
			rows, err = mon.DB.Query(query, limit)
			if err != nil {
				slog.Error("Failed to query initial raw metrics", "error", err)
				http.Error(w, "Failed to query hardware metrics", http.StatusInternalServerError)
				return lastTimestamp
			}
		} else {
			// Only rows newer than lastTimestamp (no limit; we'll send whatever new rows exist)
			query := `
				SELECT timestamp, cpu_percent, ram_percent, disk_percent
				FROM hardware_metrics
				WHERE timestamp > $1
				ORDER BY timestamp ASC;
			`
			rows, err = mon.DB.Query(query, lastTimestamp)
			if err != nil {
				slog.Error("Failed to query new raw metrics", "error", err)
				return lastTimestamp
			}
		}
	default:
		// Grouped mode using date_trunc(group, timestamp)
		// normalize group to one of allowed values to avoid injection risk
		var trunc string
		switch group {
		case "minute", "hour", "day", "month":
			trunc = group
		default:
			// fallback to minute if something unexpected came in
			trunc = "minute"
		}

		if lastTimestamp.IsZero() {
			// Initial grouped batch: latest N groups (DESC limit), then send chronologically (ASC)
			query := fmt.Sprintf(`
				SELECT ts, cpu_percent, ram_percent, disk_percent
				FROM (
					SELECT date_trunc('%s', timestamp) AS ts,
					       AVG(cpu_percent)::float8 AS cpu_percent,
					       AVG(ram_percent)::float8 AS ram_percent,
					       AVG(disk_percent)::float8 AS disk_percent
					FROM hardware_metrics
					GROUP BY date_trunc('%s', timestamp)
					ORDER BY date_trunc('%s', timestamp) DESC
					LIMIT $1
				) sub
				ORDER BY ts ASC;
			`, trunc, trunc, trunc)

			rows, err = mon.DB.Query(query, limit)
			if err != nil {
				slog.Error("Failed to query initial grouped metrics", "error", err)
				http.Error(w, "Failed to query hardware metrics", http.StatusInternalServerError)
				return lastTimestamp
			}
		} else {
			// Subsequent grouped query: aggregated groups with ts > lastTimestamp
			// Do aggregation in a subquery, then filter by ts in outer query.
			query := fmt.Sprintf(`
				SELECT ts, cpu_percent, ram_percent, disk_percent FROM (
					SELECT date_trunc('%s', timestamp) AS ts,
					       AVG(cpu_percent)::float8 AS cpu_percent,
					       AVG(ram_percent)::float8 AS ram_percent,
					       AVG(disk_percent)::float8 AS disk_percent
					FROM hardware_metrics
					GROUP BY date_trunc('%s', timestamp)
				) sub
				WHERE ts > $1
				ORDER BY ts ASC;
			`, trunc, trunc)

			rows, err = mon.DB.Query(query, lastTimestamp)
			if err != nil {
				slog.Error("Failed to query new grouped metrics", "error", err)
				return lastTimestamp
			}
		}
	}

	defer func() {
		if rows != nil {
			rows.Close()
		}
	}()

	// Iterate and stream
	var newest time.Time = lastTimestamp
	for rows.Next() {
		var metric data.HardwareMetrics
		if err := rows.Scan(&metric.Timestamp, &metric.CPUPercent, &metric.RAMPercent, &metric.DiskPercent); err != nil {
			slog.Error("Failed to scan metric row", "error", err)
			continue
		}

		// Send SSE event
		sendMetric(w, f, metric)

		// Advance newest timestamp / group key
		if metric.Timestamp.After(newest) {
			newest = metric.Timestamp
		}
	}

	if err = rows.Err(); err != nil {
		slog.Error("Error iterating metrics rows", "error", err)
	}

	return newest
}

// sendMetric marshals metric and writes an SSE data: line and flushes.
func sendMetric(w http.ResponseWriter, f http.Flusher, metric data.HardwareMetrics) {
	b, err := json.Marshal(metric)
	if err != nil {
		slog.Error("Failed to marshal metric", "error", err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
	f.Flush()
}
