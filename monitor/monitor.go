package monitor

import (
	"fmt"
	"log/slog"
	"minator/sys"
	"time"
)

func StartMonitorLoop() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		diskPercent, err := sys.DiskPercentUsage()
		var sysPercent string
		if err != nil {
			slog.Error("Could not collect disk percent", "Reason", err)
		} else {
			sysPercent = fmt.Sprintf("Disk used: %.2f%%", diskPercent)
		}
		ramPercent, err := sys.RamPercentUsage()
		if err != nil {
			slog.Error("Could not collect RAM percent", "Reason", err)
		} else {
			sysPercent += fmt.Sprintf(" RAM used: %.2f%%", ramPercent)
		}
		cpuPercent, err := sys.CpuPercentUsage()
		if err != nil {
			slog.Error("Could not collect CPU percent", "Reason", err)
		} else {
			sysPercent += fmt.Sprintf(" CPU used: %.2f%%", cpuPercent)
		}
		slog.Info("System usage", "Percentage", sysPercent)
		// TODO: we will monitor:
		// 1. nextcloud
		// 2. postgresql
		// 3. forgejo
		// 4. woodpecker-ci
		// 5. privatebin
		// 6. wireguard
	}
}
