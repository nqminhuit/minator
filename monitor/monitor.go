package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"minator/data"
	"minator/repository"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

const (
	dbName            = "minator"
	ContextTimeoutSec = 5
)

type Monitor struct {
	HTTPClient     *http.Client
	serviceStatus  repository.ServiceStatusRepo
	hardwareMetric repository.HardwareMetricsRepo
}

func NewMonitor() *Monitor {
	db, err := repository.InitDb()
	if err != nil {
		slog.Error("Failed to init database", "error", err)
		return nil
	}
	return &Monitor{
		HTTPClient:     &http.Client{Timeout: 5 * time.Second},
		serviceStatus:  repository.NewServiceStatusRepo(db),
		hardwareMetric: repository.NewHardwareMetricsRepo(db),
	}
}

func collectSystemMetrics() (float64, float64, float64) {
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

func collectHardwareMetrics() data.HardwareMetrics {
	cpuPct, ramPct, diskPct := collectSystemMetrics()
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
	if err := m.serviceStatus.InsertServiceStatus(ctx, statuses); err != nil {
		slog.Error("Failed to insert service statuses", "error", err)
	}
	metrics := collectHardwareMetrics()
	if err := m.hardwareMetric.InsertHardwareMetrics(ctx, metrics); err != nil {
		slog.Error("Failed to insert metrics", "error", err)
	}
}
