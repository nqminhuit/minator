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
		// TODO: we will monitor:
		// nextcloud
		// forgejo
		// woodpecker-ci
		// privatebin
		// wireguard
	}
}
