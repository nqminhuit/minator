package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"
)

const (
	dbName            = "minator"
	ContextTimeoutSec = 5
)

func InitDb() *sql.DB {
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

	NewServiceStatusRepo(db).CreateTableIfNotExists(ctx)
	NewHardwareMetricsRepo(db).CreateTableIfNotExists(ctx)
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
	return db
}
