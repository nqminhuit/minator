package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"time"
)

const statusFile = "data/status.json"

func formatMillis(timestamp int64) string {
	t := time.UnixMilli(timestamp)
	return t.Format("2006-01-02 15:04:05")
}

func upsertServiceStatus(filePath string, newStatus ServiceStatus) error {
	var statuses []ServiceStatus

	data, err := os.ReadFile(filePath)
	if err == nil {
		if err := json.Unmarshal(data, &statuses); err != nil {
			return fmt.Errorf("Failed to parse statuses from file: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	found := false
	for i, s := range statuses {
		if s.Name == newStatus.Name {
			statuses[i] = newStatus
			found = true
			break
		}
	}
	if !found {
		statuses = append(statuses, newStatus)
	}
	newData, err := json.Marshal(statuses)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, newData, 0644)
}

func StatusPageHandler(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open(statusFile)
	if err != nil {
		http.Error(w, "Unable to read status file", 500)
		return
	}
	defer f.Close()

	var statuses []ServiceStatus
	if err := json.NewDecoder(f).Decode(&statuses); err != nil {
		http.Error(w, "Invalid status data", 500)
		return
	}

	tmpl := template.Must(template.New("status.html").
		Funcs(template.FuncMap{"formatMillis": formatMillis}).
		ParseFiles("templates/status.html"))

	if err := tmpl.Execute(w, statuses); err != nil {
		http.Error(w, "Render error", 500)
	}
}

// While backup is in progress, it will send a curl command to this server,
// we will store health status like which backup is done, which is in progress,
// which fails, ... Then this function will update response on a json file
// so that StatusPageHandler will use this json to render the html
func ServiceStatusHandler(w http.ResponseWriter, r *http.Request) {
	var payload ServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		slog.Error("Could not decode ServiceRequest", "err", err)
		return
	}
	upsertServiceStatus(statusFile, payload.toServiceStatus(time.Now().UnixMilli()))
}
