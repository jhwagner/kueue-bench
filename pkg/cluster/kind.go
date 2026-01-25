package cluster

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jhwagner/kueue-bench/pkg/config"
	"gopkg.in/yaml.v3"
)

// TODO: Refactor to use kubernetes-sigs/e2e-framework or sigs.k8s.io/kind library
// instead of shelling out to kind CLI. This will provide better type safety,
// error handling, and eliminate the external dependency on the kind binary.

// kindClusterConfig represents a kind cluster configuration
type kindClusterConfig struct {
	Kind       string     `yaml:"kind"`
	APIVersion string     `yaml:"apiVersion"`
	Nodes      []kindNode `yaml:"nodes"`
}

type kindNode struct {
	Role string `yaml:"role"`
}

// CreateCluster creates a new kind cluster
func CreateCluster(ctx context.Context, name string, cfg *config.ClusterConfig, kubeconfigPath string) error {
	// Check if kind is installed
	if err := checkKindInstalled(); err != nil {
		return err
	}

	// Check if cluster already exists
	exists, err := clusterExists(name)
	if err != nil {
		return fmt.Errorf("failed to check if cluster exists: %w", err)
	}
	if exists {
		return fmt.Errorf("cluster '%s' already exists", name)
	}

	// Generate kind config
	kindConfig := generateKindConfig(cfg)

	// Write config to temp file
	configFile, err := writeTempKindConfig(kindConfig)
	if err != nil {
		return fmt.Errorf("failed to write kind config: %w", err)
	}
	defer os.Remove(configFile)

	// Create cluster
	fmt.Printf("Creating kind cluster '%s'...\n", name)
	cmd := exec.CommandContext(ctx, "kind", "create", "cluster",
		"--name", name,
		"--config", configFile,
		"--wait", "2m")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create kind cluster: %w", err)
	}

	// Export kubeconfig to specified path
	if err := exportKubeconfig(name, kubeconfigPath); err != nil {
		return fmt.Errorf("failed to export kubeconfig: %w", err)
	}

	fmt.Printf("✓ Cluster '%s' created successfully\n", name)
	return nil
}

// DeleteCluster deletes a kind cluster
func DeleteCluster(ctx context.Context, name string) error {
	// Check if kind is installed
	if err := checkKindInstalled(); err != nil {
		return err
	}

	// Check if cluster exists
	exists, err := clusterExists(name)
	if err != nil {
		return fmt.Errorf("failed to check if cluster exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("cluster '%s' does not exist", name)
	}

	// Delete cluster
	fmt.Printf("Deleting kind cluster '%s'...\n", name)
	cmd := exec.CommandContext(ctx, "kind", "delete", "cluster", "--name", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete kind cluster: %w", err)
	}

	fmt.Printf("✓ Cluster '%s' deleted successfully\n", name)
	return nil
}

// Helper functions

func checkKindInstalled() error {
	_, err := exec.LookPath("kind")
	if err != nil {
		return fmt.Errorf("kind is not installed or not in PATH: %w", err)
	}
	return nil
}

func clusterExists(name string) (bool, error) {
	cmd := exec.Command("kind", "get", "clusters")
	output, err := cmd.Output()
	if err != nil {
		// kind returns error if no clusters exist, which is fine
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Check if it's just "no clusters found" case
			if len(output) == 0 {
				return false, nil
			}
			return false, fmt.Errorf("failed to list kind clusters: %w (stderr: %s)", err, string(exitErr.Stderr))
		}
		return false, fmt.Errorf("failed to list kind clusters: %w", err)
	}

	// Handle empty output (no clusters)
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return false, nil
	}

	clusters := strings.Split(trimmed, "\n")
	for _, cluster := range clusters {
		if cluster == name {
			return true, nil
		}
	}
	return false, nil
}

func generateKindConfig(cfg *config.ClusterConfig) *kindClusterConfig {
	kindCfg := &kindClusterConfig{
		Kind:       "Cluster",
		APIVersion: "kind.x-k8s.io/v1alpha4",
		Nodes: []kindNode{
			{Role: "control-plane"},
		},
	}

	return kindCfg
}

func writeTempKindConfig(cfg *kindClusterConfig) (string, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal kind config: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "kind-config-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(data); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write kind config: %w", err)
	}

	return tmpFile.Name(), nil
}

func exportKubeconfig(name string, kubeconfigPath string) error {
	cmd := exec.Command("kind", "get", "kubeconfig", "--name", name)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(kubeconfigPath), 0755); err != nil {
		return fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	// Write kubeconfig to specified path
	if err := os.WriteFile(kubeconfigPath, output, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	return nil
}
