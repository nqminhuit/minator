package api

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"minator/data"
	"minator/monitor"
	"minator/repository"
	"net/http"
	"time"
)

type handler struct {
	serviceStatus  repository.ServiceStatusRepo
	hardwareMetric repository.HardwareMetricsRepo
	tmpl           *template.Template
}

func NewHandler(ss repository.ServiceStatusRepo, hm repository.HardwareMetricsRepo) *handler {
	return &handler{
		serviceStatus:  ss,
		hardwareMetric: hm,
		tmpl:           template.Must(template.ParseFiles("templates/status.html")),
	}
}

func (h *handler) StatusPageHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.tmpl.Execute(w, nil); err != nil {
		http.Error(w, "Render error", 500)
	}
}

// While backup is in progress, it will send a curl command to this server,
// we will store health status like which backup is done, which is in progress,
// which fails, ... Then this function will update response on a json file
// so that StatusPageHandler will use this json to render the html
func (m *handler) ServiceStatusHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload data.ServiceRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			slog.Error("Could not decode ServiceRequest", "err", err)
			return
		}
		statuses := []data.ServiceStatus{payload.ToHealthStatus(time.Now())}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(monitor.ContextTimeoutSec)*time.Second)
		defer cancel()
		m.serviceStatus.InsertServiceStatus(ctx, statuses)
	}
}

func (m *handler) sendStatuses(flusher http.Flusher, w http.ResponseWriter) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(monitor.ContextTimeoutSec)*time.Second)
	defer cancel()
	content, err := m.serviceStatus.GetLatestServiceStatus(ctx)
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

func (m *handler) StreamServiceStatuses(ctx context.Context) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("SSE client connected", "event", "ServiceStatus", "remote", r.RemoteAddr)
		defer slog.Info("SSE client disconnected", "event", "ServiceStatus", "remote", r.RemoteAddr)

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

		m.sendStatuses(flusher, w)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("Server disconnected")
				return
			case <-r.Context().Done():
				return // client disconnected
			case <-ticker.C:
				m.sendStatuses(flusher, w)
			}
		}
	}
}

func (h *handler) StreamHardwareMetrics(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		group := r.URL.Query().Get("group")
		if group == "" {
			group = "none"
		}
		slog.Info("SSE client connected", "event", "HardwareMetrics", "groupBy", group, "remote", r.RemoteAddr)
		defer slog.Info("SSE client disconnected", "event", "HardwareMetrics", "groupBy", group, "remote", r.RemoteAddr)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		var lastTimestamp time.Time
		lastTimestamp = h.hardwareMetric.GetMetrics(w, flusher, group, lastTimestamp)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("Server disconnected")
				return
			case <-r.Context().Done():
				slog.Info("Client disconnected from hardware metrics SSE")
				return
			case <-ticker.C:
				lastTimestamp = h.hardwareMetric.GetMetrics(w, flusher, group, lastTimestamp)
			}
		}
	}
}
