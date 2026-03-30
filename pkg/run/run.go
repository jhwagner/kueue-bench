package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	metadataDir      = ".kueue-bench/runs"
	metadataFilename = "metadata.json"
)

// Save persists run metadata to ~/.kueue-bench/runs/<runID>/metadata.json.
func Save(meta *RunMetadata) error {
	runDir, err := getRunDir(meta.RunID)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(runDir, 0750); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal run metadata: %w", err)
	}

	metadataPath := filepath.Join(runDir, metadataFilename)
	if err := os.WriteFile(metadataPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write run metadata: %w", err)
	}

	return nil
}

// Load reads run metadata from disk for the given run ID.
func Load(runID string) (*RunMetadata, error) {
	runDir, err := getRunDir(runID)
	if err != nil {
		return nil, err
	}

	metadataPath := filepath.Join(runDir, metadataFilename)
	data, err := os.ReadFile(metadataPath) //nolint:gosec // path is constructed from known base directory
	if err != nil {
		return nil, fmt.Errorf("failed to read run metadata: %w", err)
	}

	var meta RunMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal run metadata: %w", err)
	}

	return &meta, nil
}

// List returns all saved run metadata, sorted by StartedAt descending (newest first).
func List() ([]*RunMetadata, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	runsDir := filepath.Join(home, metadataDir)
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*RunMetadata{}, nil
		}
		return nil, fmt.Errorf("failed to read runs directory: %w", err)
	}

	var runs []*RunMetadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		meta, err := Load(entry.Name())
		if err != nil {
			continue
		}

		runs = append(runs, meta)
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})

	return runs, nil
}

func getRunDir(runID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, metadataDir, runID), nil
}
