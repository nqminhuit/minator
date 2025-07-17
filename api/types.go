package api

import (
	"encoding/json"
	"log/slog"
)

type ServiceStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	LastCheck int64  `json:"lastCheck"`
	Message   string `json:"message,omitempty"`
}

type ServiceRequest struct {
	Name    string         `json:"name"`
	Status  string         `json:"status"`
	Details map[string]any `json:"details"`
}

func (s *ServiceRequest) toServiceStatus(lastCheck int64) ServiceStatus {
	msg, err := json.Marshal(s.Details)
	if err != nil {
		slog.Error("Could not convert ServiceRequest#Details to json", "err", err)
		return ServiceStatus{}
	}
	return ServiceStatus{
		Name:      s.Name,
		Status:    s.Status,
		LastCheck: lastCheck,
		Message:   string(msg),
	}
}
