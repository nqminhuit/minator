package monitor

import (
	"minator/data"
	"os/exec"
	"strings"
	"time"
)

const container = "hl-postgres"

func postgresStatus() data.ServiceStatus {
	serviceStatus := data.ServiceStatus{
		Name:      "PostgreSQL",
		LastCheck: time.Now().UnixMilli(),
	}
	cmd := exec.Command("podman", "healthcheck", "run", container)
	output, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(output))
	if err == nil && outputStr == "" {
		serviceStatus.Status = "healthy"
		return serviceStatus
	}
	serviceStatus.Status = outputStr
	serviceStatus.Message = "Service unhealthy"
	return serviceStatus
}
