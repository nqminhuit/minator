package api

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"minator/data"
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
func ServiceStatusHandler(w http.ResponseWriter, r *http.Request) {
	var payload data.ServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		slog.Error("Could not decode ServiceRequest", "err", err)
		return
	}
	data.UpsertServiceStatus(payload.ToServiceStatus(time.Now().UnixMilli()))
}

func EventsHandler(w http.ResponseWriter, r *http.Request) {
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

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return // client disconnected
		case <-ticker.C:
			// Read status from JSON file
			content, err := data.ReadServiceStatusContent()
			if len(content) > 10*1024*1024 {
				fmt.Fprintf(w, "event: error\ndata: status file too large\n\n")
				flusher.Flush()
				continue
			}
			if err != nil {
				slog.Error("Failed to read service status file", "err", err)
				fmt.Fprintf(w, "event: error\ndata: %v\n\n", err)
				flusher.Flush()
				continue
			}
			// Write event to stream
			fmt.Fprintf(w, "event: update\ndata: %s\n\n", content)
			flusher.Flush()
		}
	}
}
