package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"minator/data"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

const (
	dbName             = "minator"
	tblHardwareMetrics = "hardware_metrics"
	tblServiceStatus   = "service_status"
	ContextTimeoutSec  = 5
)

type Monitor struct {
	HTTPClient *http.Client
	DB         *sql.DB
}

func NewMonitor() *Monitor {
	dsn := fmt.Sprintf(
		"host=localhost port=5432 user=postgres password=%s dbname=%s sslmode=disable",
		os.Getenv("POSTGRES_PASSWORD"), dbName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		slog.Error("Failed to connect to PostgreSQL", "error", err)
		panic(err)
	}
	if err := db.Ping(); err != nil {
		slog.Error("PostgreSQL ping failed", "error", err)
		panic(err)
	}
	// Create minator user and assign password
	minatorPassword := os.Getenv("MINATOR_DB_PASSWORD")
	if minatorPassword == "" {
		err := "MINATOR_DB_PASSWORD environment variable not set"
		slog.Error(err)
		panic(err)
	}
	// Update password if user exists
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ContextTimeoutSec)*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER ROLE minator WITH ENCRYPTED PASSWORD '%s'", minatorPassword))
	if err != nil {
		slog.Error("Failed to update minator user password", "error", err)
		panic(err)
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
			timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(50) NOT NULL,
			detail TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_service_status_timestamp_desc ON %s (timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_name ON %s (name);
		COMMENT ON TABLE %s IS 'Stores services health status on homelab';
		GRANT ALL ON %s TO minator;
		GRANT USAGE, SELECT ON SEQUENCE service_status_id_seq TO minator;`,
		tblServiceStatus, tblServiceStatus, tblServiceStatus, tblServiceStatus, tblServiceStatus)); err != nil {
		slog.Error("Failed to create table", "tableName", tblServiceStatus, "error", err)
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf(`
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
	db.Close()

	// Connect as minator user for normal operations
	userDSN := fmt.Sprintf(
		"host=localhost port=5432 user=minator password=%s dbname=%s sslmode=disable",
		minatorPassword, dbName)
	db, err = sql.Open("postgres", userDSN)
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	if err != nil {
		slog.Error("Failed to connect to PostgreSQL as minator user", "error", err)
	}
	if err := db.Ping(); err != nil {
		slog.Error("PostgreSQL ping failed as minator user", "error", err)
	}
	return &Monitor{
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		DB:         db,
	}
}

func (m *Monitor) collectSystemMetrics() (float64, float64, float64) {
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		slog.Error("Failed to collect CPU metric", "error", err)
	}
	vm, err := mem.VirtualMemory()
	if err != nil {
		slog.Error("Failed to collect RAM metric", "error", err)
	}
	diskUsage, err := disk.Usage("/")
	if err != nil {
		slog.Error("Failed to collect DISK metric", "error", err)
	}
	return cpuPercent[0], vm.UsedPercent, diskUsage.UsedPercent
}

func (m *Monitor) checkHttpHealth(url string) data.ServiceStatus {
	resp, err := m.HTTPClient.Get(url)
	if err != nil {
		return data.ServiceStatus{
			Status: "unhealthy",
			Detail: fmt.Sprintf("HTTP check failed: %v", err),
		}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("Could not close response body", "error", err)
		}
	}()
	if resp.StatusCode != 200 {
		return data.ServiceStatus{
			Status: "unhealthy",
			Detail: fmt.Sprintf("Unexpected HTTP status: %d", resp.StatusCode),
		}
	}
	if url == "http://localhost/nextcloud/status.php" {
		var status map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&status); err == nil && status["status"] == "ok" {
			return data.ServiceStatus{Status: "healthy", Detail: "Nextcloud OK"}
		}
		return data.ServiceStatus{Status: "unhealthy", Detail: "Nextcloud status not OK"}
	}
	return data.ServiceStatus{Status: "healthy", Detail: "HTTP 200 OK"}
}

func (m *Monitor) CheckPodmanHealth(container string) data.ServiceStatus {
	for i := 1; i <= 3; i++ {
		cmd := exec.Command("podman", "healthcheck", "run", container)
		output, err := cmd.CombinedOutput()
		if err == nil && strings.TrimSpace(string(output)) == "" {
			return data.ServiceStatus{Status: "healthy", Detail: fmt.Sprintf("%s healthcheck OK", container)}
		}
		time.Sleep(time.Duration(i) * time.Second)
	}
	return data.ServiceStatus{Status: "unhealthy", Detail: fmt.Sprintf("%s healthcheck failed", container)}
}

func (m *Monitor) CheckWireGuardHealth() data.ServiceStatus {
	conn, err := net.DialTimeout("udp", "10.0.0.1:51820", 5*time.Second) // WireGuard peer
	if err != nil {
		return data.ServiceStatus{Status: "unhealthy", Detail: fmt.Sprintf("WireGuard connection failed: %v", err)}
	}
	conn.Close()
	return data.ServiceStatus{Status: "healthy", Detail: "WireGuard OK"}
}

func (m *Monitor) collectHardwareMetrics() data.HardwareMetrics {
	cpuPct, ramPct, diskPct := m.collectSystemMetrics()
	return data.HardwareMetrics{
		CPUPercent:  cpuPct,
		RAMPercent:  ramPct,
		DiskPercent: diskPct,
		Timestamp:   time.Now(),
	}
}

func (m *Monitor) collectServiceStatus() []data.ServiceStatus {
	var statuses []data.ServiceStatus
	checks := map[string]func() data.ServiceStatus{
		"forgejo":    func() data.ServiceStatus { return m.checkHttpHealth("http://localhost:3000/api/healthz") },
		"privatebin": func() data.ServiceStatus { return m.checkHttpHealth("http://localhost:8080/") },
		"postgresql": func() data.ServiceStatus { return m.CheckPodmanHealth("hl-postgres") },
		"wireguard":  m.CheckWireGuardHealth,
		// "nextcloud":   func() HealthStatus { return m.CheckHTTPHealth("http://localhost/nextcloud/status.php") },
	}
	for name, check := range checks {
		status := check()
		status.Name = name
		status.Timestamp = time.Now()
		statuses = append(statuses, status)
	}
	return statuses
}

func (m *Monitor) InsertServiceStatus(ctx context.Context, statuses []data.ServiceStatus) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
		INSERT INTO %s (timestamp, name, status, detail)
		VALUES ($1, $2, $3, $4)`,
		tblServiceStatus))
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, s := range statuses {
		if _, err := stmt.ExecContext(ctx, s.Timestamp, s.Name, s.Status, s.Detail); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (m *Monitor) InsertHardwareMetrics(ctx context.Context, s data.HardwareMetrics) error {
	_, err := m.DB.ExecContext(ctx, `
		INSERT INTO `+tblHardwareMetrics+` (timestamp, cpu_percent, ram_percent, disk_percent) VALUES ($1, $2, $3, $4)`,
		s.Timestamp, s.CPUPercent, s.RAMPercent, s.DiskPercent)
	return err
}

func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	m.collectMetrics()
	for {
		select {
		case <-ctx.Done():
			slog.Info("Stop monitoring due to context cancellation.")
			return
		case <-ticker.C:
			m.collectMetrics()
		}
	}
}

func (m *Monitor) collectMetrics() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ContextTimeoutSec)*time.Second)
	defer cancel()
	statuses := m.collectServiceStatus()
	if err := m.InsertServiceStatus(ctx, statuses); err != nil {
		slog.Error("Failed to insert service statuses", "error", err)
	}
	metrics := m.collectHardwareMetrics()
	if err := m.InsertHardwareMetrics(ctx, metrics); err != nil {
		slog.Error("Failed to insert metrics", "error", err)
	}
}

func (m *Monitor) GetLatestServiceStatus(ctx context.Context) ([]data.ServiceStatus, error) {
	rows, err := m.DB.QueryContext(ctx, `
		WITH LatestStatus AS (
			SELECT name, MAX(timestamp) AS max_timestamp
			FROM `+tblServiceStatus+`
			GROUP BY name
		)
		SELECT m.name, m.status, m.detail, m.timestamp
		FROM `+tblServiceStatus+` m
		INNER JOIN LatestStatus ls ON m.name = ls.name AND m.timestamp = ls.max_timestamp
		ORDER BY m.name;`)
	if err != nil {
		slog.Error("Failed to query metrics", "error", err)
		return nil, err
	}
	defer rows.Close()
	var statuses []data.ServiceStatus
	for rows.Next() {
		var s data.ServiceStatus
		if err := rows.Scan(&s.Name, &s.Status, &s.Detail, &s.Timestamp); err != nil {
			slog.Error("Failed to scan service status", "error", err)
			return nil, err
		}
		statuses = append(statuses, s)
	}
	return statuses, nil
}
