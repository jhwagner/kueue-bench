package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// loadYAML reads a YAML file and unmarshals it into a value of type T.
func loadYAML[T any](path, typeName string) (*T, error) {
	data, err := os.ReadFile(path) //nolint:gosec // filepath is user-provided CLI input, not untrusted
	if err != nil {
		return nil, fmt.Errorf("failed to read %s file: %w", typeName, err)
	}

	var result T
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse %s YAML: %w", typeName, err)
	}

	return &result, nil
}

// LoadTopology loads and parses a topology configuration file
func LoadTopology(path string) (*Topology, error) {
	return loadYAML[Topology](path, "topology")
}

// LoadWorkloadProfile loads and parses a workload profile configuration file
func LoadWorkloadProfile(path string) (*WorkloadProfile, error) {
	return loadYAML[WorkloadProfile](path, "workload profile")
}
