package monitor

import (
	"minator/data"
	"time"
)

func StartMonitorLoop() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		data.UpsertServiceStatus(hardwareStatus())
		data.UpsertServiceStatus(postgresStatus())
		data.UpsertServiceStatus(forgejoStatus())
		data.UpsertServiceStatus(pbStatus())
		// TODO: we will monitor:
		// nextcloud
		// wireguard
	}
}
