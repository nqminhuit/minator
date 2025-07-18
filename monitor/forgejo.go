package monitor

import (
	"fmt"
	"minator/data"
	"net/http"
	"time"
)

func forgejoStatus() data.ServiceStatus {
	serviceStatus := data.ServiceStatus{
		Name:      "Forgejo",
		LastCheck: time.Now().UnixMilli(),
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:3000/user/login")
	if err != nil {
		serviceStatus.Status = "down"
		serviceStatus.Message = "HTTP error: " + err.Error()
		return serviceStatus
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		serviceStatus.Status = "healthy"
	} else {
		serviceStatus.Status = "down"
		serviceStatus.Message = fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
	}
	return serviceStatus
}
