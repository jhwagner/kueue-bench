package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadTopology loads and parses a topology configuration file
func LoadTopology(filepath string) (*Topology, error) {
	data, err := os.ReadFile(filepath) //nolint:gosec // filepath is user-provided CLI input, not untrusted
	if err != nil {
		return nil, fmt.Errorf("failed to read topology file: %w", err)
	}

	var topology Topology
	if err := yaml.Unmarshal(data, &topology); err != nil {
		return nil, fmt.Errorf("failed to parse topology YAML: %w", err)
	}

	return &topology, nil
}
