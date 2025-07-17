package api

import (
	"fmt"
	"strings"
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
	msg := make([]string, 0, len(s.Details))
	for key, val := range s.Details {
		msg = append(msg, fmt.Sprintf("%s: %s", key, val))
	}
	return ServiceStatus{
		Name:      s.Name,
		Status:    s.Status,
		LastCheck: lastCheck,
		Message:   strings.Join(msg, ", "),
	}
}
