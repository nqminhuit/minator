package api

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"minator/data"
	"net/http"
	"time"
)

func formatMillis(timestamp int64) string {
	t := time.UnixMilli(timestamp)
	return t.Format("2006-01-02 15:04:05")
}

func StatusPageHandler(w http.ResponseWriter, r *http.Request) {
	statuses, err := data.GetServiceStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	var payload data.ServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		slog.Error("Could not decode ServiceRequest", "err", err)
		return
	}
	data.UpsertServiceStatus(payload.ToServiceStatus(time.Now().UnixMilli()))
}
