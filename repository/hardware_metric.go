package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"minator/data"
	"net/http"
	"time"
)

const tblHardwareMetrics = "hardware_metrics"

type HardwareMetricsRepo interface {
	GetMetrics(w http.ResponseWriter, f http.Flusher, group string, lastTimestamp time.Time) time.Time
	InsertHardwareMetrics(ctx context.Context, s data.HardwareMetrics) error
	CreateTableIfNotExists(ctx context.Context)
}

type hardwareMetricsRepo struct {
	db *sql.DB
}

func NewHardwareMetricsRepo(db *sql.DB) HardwareMetricsRepo {
	return &hardwareMetricsRepo{db: db}
}

// limit based on group
func calculateLimit(group string) int {
	switch group {
	case "minute":
		return 200 // 100 minute samples
	case "hour":
		return 200 * 60 // 100 hour samples
	case "day":
		return 200 * 60 * 24 // 100 day samples
	case "month":
		return 200 * 60 * 24 * 30 // 3000 day samples
	default:
		return 100 // 100 second samples
	}
}

func (h *hardwareMetricsRepo) GetMetrics(w http.ResponseWriter, f http.Flusher, group string, lastTimestamp time.Time) time.Time {
	var (
		rows *sql.Rows
		err  error
	)

	limit := calculateLimit(group)

	switch group {
	case "none":
		if lastTimestamp.IsZero() {
			// Initial raw batch: get latest N rows, then send in chronological order
			query := `
				SELECT timestamp, cpu_percent, ram_percent, disk_percent
				FROM (
					SELECT timestamp, cpu_percent, ram_percent, disk_percent
					FROM hardware_metrics
					ORDER BY timestamp DESC
					LIMIT $1
				) sub
				ORDER BY timestamp ASC;
			`
			rows, err = h.db.Query(query, limit)
			if err != nil {
				slog.Error("Failed to query initial raw metrics", "error", err)
				fmt.Fprintf(w, "event: error\ndata: %v\n\n", err)
				f.Flush()
				return lastTimestamp
			}
		} else {
			// Only rows newer than lastTimestamp (no limit; we'll send whatever new rows exist)
			query := `
				SELECT timestamp, cpu_percent, ram_percent, disk_percent
				FROM hardware_metrics
				WHERE timestamp > $1
				ORDER BY timestamp ASC;
			`
			rows, err = h.db.Query(query, lastTimestamp)
			if err != nil {
				slog.Error("Failed to query new raw metrics", "error", err)
				fmt.Fprintf(w, "event: error\ndata: %v\n\n", err)
				f.Flush()
				return lastTimestamp
			}
		}
	case "minute", "hour", "day", "month":
		if lastTimestamp.IsZero() {
			query := fmt.Sprintf(`
				SELECT date_trunc('%s', timestamp) AS ts,
					AVG(cpu_percent)::float8 AS cpu_percent,
					AVG(ram_percent)::float8 AS ram_percent,
					AVG(disk_percent)::float8 AS disk_percent
				FROM (
					SELECT timestamp, cpu_percent, ram_percent, disk_percent
					FROM hardware_metrics
					ORDER BY timestamp DESC
					LIMIT $1
				) recent
				GROUP BY ts
				ORDER BY ts ASC;
			`, group)

			rows, err = h.db.Query(query, limit)
			if err != nil {
				slog.Error("Failed to query initial grouped metrics", "error", err)
				http.Error(w, "Failed to query hardware metrics", http.StatusInternalServerError)
				return lastTimestamp
			}
		} else {
			// Subsequent grouped query: aggregated groups with ts > lastTimestamp
			// Do aggregation in a subquery, then filter by ts in outer query.
			query := fmt.Sprintf(`
				SELECT ts, cpu_percent, ram_percent, disk_percent
				FROM (
					SELECT date_trunc('%s', timestamp) AS ts_trunc,
						MAX(timestamp) AS ts,
						AVG(cpu_percent)::float8 AS cpu_percent,
						AVG(ram_percent)::float8 AS ram_percent,
						AVG(disk_percent)::float8 AS disk_percent
					FROM hardware_metrics
					WHERE timestamp > $1
					GROUP BY ts_trunc
				) sub
				WHERE ts_trunc > $1
				ORDER BY ts_trunc ASC;
				`, group)

			rows, err = h.db.Query(query, lastTimestamp)
			if err != nil {
				slog.Error("Failed to query new grouped metrics", "error", err)
				return lastTimestamp
			}
		}
	}

	defer func() {
		if rows != nil {
			rows.Close()
		}
	}()

	// Iterate and stream
	newest := lastTimestamp
	for rows.Next() {
		var metric data.HardwareMetrics
		if err := rows.Scan(&metric.Timestamp, &metric.CPUPercent, &metric.RAMPercent, &metric.DiskPercent); err != nil {
			slog.Error("Failed to scan metric row", "error", err)
			continue
		}

		// Send SSE event
		sendMetric(w, f, metric)

		if metric.Timestamp.After(newest) {
			newest = metric.Timestamp
		}
	}

	if err = rows.Err(); err != nil {
		slog.Error("Error iterating metrics rows", "error", err)
	}

	return newest
}

// sendMetric marshals metric and writes an SSE data: line and flushes.
func sendMetric(w http.ResponseWriter, f http.Flusher, metric data.HardwareMetrics) {
	b, err := json.Marshal(metric)
	if err != nil {
		slog.Error("Failed to marshal metric", "error", err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
	f.Flush()
}

func (m *hardwareMetricsRepo) InsertHardwareMetrics(ctx context.Context, s data.HardwareMetrics) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO `+tblHardwareMetrics+` (timestamp, cpu_percent, ram_percent, disk_percent) VALUES ($1, $2, $3, $4)`,
		s.Timestamp, s.CPUPercent, s.RAMPercent, s.DiskPercent)
	return err
}

func (m *hardwareMetricsRepo) CreateTableIfNotExists(ctx context.Context) {
	if _, err := m.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
			timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			cpu_percent FLOAT NOT NULL,
			ram_percent FLOAT NOT NULL,
			disk_percent FLOAT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_hardware_metrics_timestamp_desc ON %s (timestamp DESC);
		COMMENT ON TABLE %s IS 'Stores hardware metrics on homelab';
		GRANT ALL ON %s TO minator;
		GRANT USAGE, SELECT ON SEQUENCE hardware_metrics_id_seq TO minator;`,
		tblHardwareMetrics, tblHardwareMetrics, tblHardwareMetrics, tblHardwareMetrics)); err != nil {
		slog.Error("Failed to create table", "tableName", tblHardwareMetrics, "error", err)
	}
}
