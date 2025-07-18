package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const statusFile = "/tmp/status.json"

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
