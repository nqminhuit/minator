package api

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
)

type ServiceStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	LastCheck string `json:"last_check"`
	Message   string `json:"message,omitempty"`
}

func StatusPageHandler(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open("data/status.json")
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

	tmpl := template.Must(template.ParseFiles("templates/status.html"))
	if err := tmpl.Execute(w, statuses); err != nil {
		http.Error(w, "Render error", 500)
	}
}

// While backup is in progress, it will send a curl command to this server,
// we will store health status like which backup is done, which is in progress,
// which fails, ... Then this function will update response on a json file
// so that StatusPageHandler will use this json to render the html
func BackupStatusHandler(w http.ResponseWriter, r *http.Request) {

}
