package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const statusFile = "data/status.json"

func GetServiceStatus() ([]ServiceStatus, error) {
	f, err := os.Open(statusFile)
	if err != nil {
		return nil, fmt.Errorf("Unable to read status file")
	}
	defer f.Close()
	var statuses []ServiceStatus
	if err := json.NewDecoder(f).Decode(&statuses); err != nil {
		return nil, fmt.Errorf("Invalid status data")
	}
	return statuses, nil

func ReadServiceStatusContent() ([]byte, error) {
	return os.ReadFile(statusFile)
}

func UpsertServiceStatus(newStatus ServiceStatus) error {
	var statuses []ServiceStatus

	data, err := os.ReadFile(statusFile)
	if err == nil {
		if err := json.Unmarshal(data, &statuses); err != nil {
			return fmt.Errorf("Failed to parse statuses from file: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	found := false
	for i, s := range statuses {
		if s.Name == newStatus.Name {
			statuses[i] = newStatus
			found = true
			break
		}
	}
	if !found {
		statuses = append(statuses, newStatus)
	}
	newData, err := json.Marshal(statuses)
	if err != nil {
		return err
	}
	return os.WriteFile(statusFile, newData, 0644)
}
