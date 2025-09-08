package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
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
	dbName    = "minator"
	tableName = "metrics"
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
	_, err = db.Exec(fmt.Sprintf("ALTER ROLE minator WITH ENCRYPTED PASSWORD '%s'", minatorPassword))
	if err != nil {
		slog.Error("Failed to update minator user password", "error", err)
		panic(err)
	}

	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
			timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(50) NOT NULL,
			detail TEXT,
			cpu_percent FLOAT NOT NULL,
			ram_percent FLOAT NOT NULL,
			disk_percent FLOAT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_timestamp ON %s (timestamp);
		CREATE INDEX IF NOT EXISTS idx_name ON %s (name);
		COMMENT ON TABLE %s IS 'Stores health status and system metrics for Minator services';
		GRANT ALL ON %s TO minator;
		GRANT USAGE, SELECT ON SEQUENCE metrics_id_seq TO minator;`,
		tableName, tableName, tableName, tableName, tableName)); err != nil {
		slog.Error("Failed to create table", "tableName", tableName, "error", err)
	}
	db.Close()

	// Connect as minator user for normal operations
	userDSN := fmt.Sprintf(
		"host=localhost port=5432 user=minator password=%s dbname=%s sslmode=disable",
		minatorPassword, dbName)
	db, err = sql.Open("postgres", userDSN)
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
	cpuPercent, _ := cpu.Percent(0, false)
	vm, _ := mem.VirtualMemory()
	diskUsage, _ := disk.Usage("/")
	return cpuPercent[0], vm.UsedPercent, diskUsage.UsedPercent
}

func (m *Monitor) CheckHTTPHealth(url string) data.HealthStatus {
	resp, err := m.HTTPClient.Get(url)
	if err != nil || resp.StatusCode != 200 {
		return data.HealthStatus{Status: "unhealthy", Detail: fmt.Sprintf("HTTP check failed: %v", err)}
	}
	if url == "http://localhost/nextcloud/status.php" {
		var status map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&status); err == nil && status["status"] == "ok" {
			return data.HealthStatus{Status: "healthy", Detail: "Nextcloud OK"}
		}
		return data.HealthStatus{Status: "unhealthy", Detail: "Nextcloud status not OK"}
	}
	return data.HealthStatus{Status: "healthy", Detail: "HTTP 200 OK"}
}

func (m *Monitor) CheckPodmanHealth(container string) data.HealthStatus {
	for i := range 3 {
		cmd := exec.Command("podman", "healthcheck", "run", container)
		output, err := cmd.CombinedOutput()
		if err == nil && strings.TrimSpace(string(output)) == "" {
			return data.HealthStatus{Status: "healthy", Detail: fmt.Sprintf("%s healthcheck OK", container)}
		}
		time.Sleep(time.Duration(i) * time.Second)
	}
	return data.HealthStatus{Status: "unhealthy", Detail: fmt.Sprintf("%s healthcheck failed", container)}
}

func (m *Monitor) CheckWireGuardHealth() data.HealthStatus {
	conn, err := net.DialTimeout("udp", "10.0.0.1:51820", 5*time.Second) // WireGuard peer
	if err != nil {
		return data.HealthStatus{Status: "unhealthy", Detail: fmt.Sprintf("WireGuard connection failed: %v", err)}
	}
	conn.Close()
	return data.HealthStatus{Status: "healthy", Detail: "WireGuard OK"}
}

func (m *Monitor) collectMetrics() []data.HealthStatus {
	var statuses []data.HealthStatus
	checks := map[string]func() data.HealthStatus{
		"forgejo":    func() data.HealthStatus { return m.CheckHTTPHealth("http://localhost:3000/api/healthz") },
		"privatebin": func() data.HealthStatus { return m.CheckHTTPHealth("http://localhost:8080/") },
		"postgresql": func() data.HealthStatus { return m.CheckPodmanHealth("hl-postgres") },
		"wireguard":  m.CheckWireGuardHealth,
		// "nextcloud":   func() HealthStatus { return m.CheckHTTPHealth("http://localhost/nextcloud/status.php") },
	}
	cpuPct, ramPct, diskPct := m.collectSystemMetrics()
	for name, check := range checks {
		status := check()
		status.Name = name
		status.CPUPercent = cpuPct
		status.RAMPercent = ramPct
		status.DiskPercent = diskPct
		status.Timestamp = time.Now()
		statuses = append(statuses, status)
	}
	return statuses
}

func (m *Monitor) InsertMetrics(statuses []data.HealthStatus) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO metrics (timestamp, cpu_percent, ram_percent, disk_percent, name, status, detail) VALUES ($1, $2, $3, $4, $5, $6, $7)")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, s := range statuses {
		if _, err := stmt.Exec(s.Timestamp, s.CPUPercent, s.RAMPercent, s.DiskPercent, s.Name, s.Status, s.Detail); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("Stop monitoring due to context cancellation.")
			return
		case <-ticker.C:
			statuses := m.collectMetrics()
			if err := m.InsertMetrics(statuses); err != nil {
				slog.Error("Failed to insert metrics", "error", err)
			}
		}
	}
}

func (m *Monitor) Get(limit int) ([]data.HealthStatus, error) {
	q := fmt.Sprintf(`
		SELECT timestamp, cpu_percent, ram_percent, disk_percent, name, status, detail
		FROM %s
		ORDER BY timestamp
		DESC LIMIT %d`,
		tableName, limit)
	rows, err := m.DB.Query(q)
	if err != nil {
		slog.Error("Failed to query metrics", "error", err)
		return nil, err
	}
	defer rows.Close()
	var statuses []data.HealthStatus
	for rows.Next() {
		var s data.HealthStatus
		if err := rows.Scan(&s.Timestamp, &s.CPUPercent, &s.RAMPercent, &s.DiskPercent, &s.Name, &s.Status, &s.Detail); err != nil {
			slog.Error("Failed to scan metrics", "error", err)
			return nil, err
		}
		statuses = append(statuses, s)
	}
	return statuses, nil
}

func (m *Monitor) StatusHandler(w http.ResponseWriter, r *http.Request) {
	statuses, err := m.Get(10)
	if err != nil {
		http.Error(w, "Failed to query metrics", http.StatusInternalServerError)
		return
	}
	tmpl := template.Must(template.ParseFiles("status.html"))
	tmpl.Execute(w, struct{ Statuses []data.HealthStatus }{Statuses: statuses})
}
