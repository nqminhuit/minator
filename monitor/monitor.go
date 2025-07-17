package monitor

import (
	"log/slog"
	"time"
)

func StartMonitorLoop() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		slog.Info("monitoring...")
	}
}
