package monitor

import (
	"log/slog"
	"minator/data"
	"minator/sys"
	"time"
)

func constructStatusMessage(diskPercent, ramPercent, cpuPercent float32) (status string, msg string) {
	status = "healthy"
	msg = ""
	if diskPercent > 90 {
		status = "critical"
		msg = "Running out of disk space"
		return
	}
	if diskPercent > 70 {
		status = "degraded"
		msg = "Disk usage is high"
		return
	}
	if cpuPercent > 90 {
		status = "critical"
		msg = "CPU usage is extremely high"
		return
	}
	if cpuPercent > 70 {
		status = "degraded"
		msg = "CPU usage is high"
		return
	}
	if ramPercent > 90 {
		status = "critical"
		msg = "RAM usage is extremely high"
		return
	}
	if ramPercent > 70 {
		status = "degraded"
		msg = "RAM usage is high"
		return
	}
	return
}

func hardwareStatus() data.ServiceStatus {
	var status, msg string
	diskPercent, err := sys.DiskPercentUsage()
	if err != nil {
		status = "critical"
		msg = "Could not collect disk percent"
		slog.Error(msg, "Reason", err)
	}
	ramPercent, err := sys.RamPercentUsage()
	if err != nil {
		status = "critical"
		msg = "Could not collect RAM percent"
		slog.Error(msg, "Reason", err)
	}
	cpuPercent, err := sys.CpuPercentUsage()
	if err != nil {
		status = "critical"
		msg = "Could not collect CPU percent"
		slog.Error(msg, "Reason", err)
	}
	if status != "critical" {
		status, msg = constructStatusMessage(diskPercent, ramPercent, cpuPercent)
	}
	return data.ServiceStatus{
		Name:      "Hardware",
		Status:    status,
		LastCheck: time.Now().UnixMilli(),
		Message:   msg,
	}
}
