package monitor

import (
	"context"
	"database/sql"
	"minator/data"
	"time"

	_ "github.com/lib/pq"
)

func postgresStatus() data.ServiceStatus {
	serviceStatus := data.ServiceStatus{
		Name:      "PostgreSQL",
		LastCheck: time.Now().UnixMilli(),
	}
	db, err := sql.Open("postgres", "host=localhost port=5432 user=postgres password=123456 sslmode=disable")
	if err != nil {
		serviceStatus.Status = "down"
		serviceStatus.Message = "connection failed: " + err.Error()
		return serviceStatus
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		serviceStatus.Status = "down"
		serviceStatus.Message = "ping failed: " + err.Error()
	} else {
		serviceStatus.Status = "healthy"
	}
	return serviceStatus
}
