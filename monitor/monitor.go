package monitor

import (
	"log/slog"
	"time"
)

func StartMonitorLoop() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		slog.Info("monitoring...")
		// TODO: we will monitor:
		// 1. nextcloud
		// 2. postgresql
		// 3. forgejo
		// 4. woodpecker-ci
		// 5. privatebin
		// 6. wireguard
	}
}
