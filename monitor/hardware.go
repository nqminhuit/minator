package monitor

import (
	"log/slog"
	"minator/data"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

func constructStatusMessage(diskUsage *disk.UsageStat, ram *mem.VirtualMemoryStat, cpus []float64) (status string, msg string) {
	status = "healthy"
	msg = ""
	diskPercent := diskUsage.UsedPercent
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
	cpuPercent := cpus[0]
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
	ramPercent := ram.UsedPercent
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
	diskUsage, err := disk.Usage("/")
	if err != nil {
		status = "critical"
		msg = "Could not collect disk percent"
		slog.Error(msg, "Reason", err)
	}
	ram, err := mem.VirtualMemory()
	if err != nil {
		status = "critical"
		msg = "Could not collect RAM percent"
		slog.Error(msg, "Reason", err)
	}
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		status = "critical"
		msg = "Could not collect CPU percent"
		slog.Error(msg, "Reason", err)
	}
	// network, err := net.IOCounters(false)
	// if err != nil {
	// 	status = "critical"
	// 	msg = "Could not collect Network stats"
	// 	slog.Error(msg, "Reason", err)
	// }
	// slog.Info("network stats", "", network)
	if status != "critical" {
		status, msg = constructStatusMessage(diskUsage, ram, cpuPercent)
	}
	return data.ServiceStatus{
		Name:      "Hardware",
		Status:    status,
		LastCheck: time.Now().UnixMilli(),
		Message:   msg,
	}
}
