package data

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ServiceStatus struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Detail      string    `json:"detail"`
	Timestamp   time.Time `json:"timestamp"`
}

type HardwareMetrics struct {
	CPUPercent  float64   `json:"cpu_percent"`
	RAMPercent  float64   `json:"ram_percent"`
	DiskPercent float64   `json:"disk_percent"`
	Timestamp   time.Time `json:"timestamp"`
}

type ServiceRequest struct {
	Name    string         `json:"name"`
	Status  string         `json:"status"`
	Details map[string]any `json:"details"`
}

func (s *ServiceRequest) ToHealthStatus(lastCheck time.Time) ServiceStatus {
	msg := make([]string, 0, len(s.Details))
	for key, val := range s.Details {
		msg = append(msg, fmt.Sprintf("%s: %s", key, val))
	}
	return ServiceStatus{
		Name:      s.Name,
		Status:    s.Status,
		Timestamp: lastCheck,
		Detail:    strings.Join(msg, ", "),
	}
}

func ServiceStatusToJSON(status []ServiceStatus) (string, error) {
	data, err := json.Marshal(status)
	if err != nil {
		return "", fmt.Errorf("failed to marshal HealthStatus: %v", err)
	}
	return string(data), nil
}
