package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"minator/data"
)

const TblServiceStatus = "service_status"

type ServiceStatusRepo interface {
	InsertServiceStatus(ctx context.Context, statuses []data.ServiceStatus) error
	GetLatestServiceStatus(ctx context.Context) ([]data.ServiceStatus, error)
	CreateTableIfNotExists(ctx context.Context)
}

type serviceStatusRepo struct {
	db *sql.DB
}

func NewServiceStatusRepo(db *sql.DB) ServiceStatusRepo {
	return &serviceStatusRepo{
		db: db,
	}
}

func (m *serviceStatusRepo) InsertServiceStatus(ctx context.Context, statuses []data.ServiceStatus) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
		INSERT INTO %s (timestamp, name, status, detail)
		VALUES ($1, $2, $3, $4)`,
		TblServiceStatus))
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

func (m *serviceStatusRepo) GetLatestServiceStatus(ctx context.Context) ([]data.ServiceStatus, error) {
	rows, err := m.db.QueryContext(ctx, `
		WITH LatestStatus AS (
			SELECT name, MAX(timestamp) AS max_timestamp
			FROM `+TblServiceStatus+`
			GROUP BY name
		)
		SELECT m.name, m.status, m.detail, m.timestamp
		FROM `+TblServiceStatus+` m
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

func (m *serviceStatusRepo) CreateTableIfNotExists(ctx context.Context) {
	if _, err := m.db.ExecContext(ctx, fmt.Sprintf(`
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
		TblServiceStatus,
		TblServiceStatus,
		TblServiceStatus,
		TblServiceStatus,
		TblServiceStatus)); err != nil {
		slog.Error("Failed to create table", "tableName", TblServiceStatus, "error", err)
	}
}
