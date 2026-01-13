package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jhwagner/kueue-bench/pkg/config"
	"gopkg.in/yaml.v3"
)

// TODO: Refactor to use kubernetes-sigs/e2e-framework or sigs.k8s.io/kind library
// instead of shelling out to kind CLI. This will provide better type safety,
// error handling, and eliminate the external dependency on the kind binary.

const (
	metadataDir = ".kueue-bench/clusters"
)

// kindClusterConfig represents a kind cluster configuration
type kindClusterConfig struct {
	Kind       string     `yaml:"kind"`
	APIVersion string     `yaml:"apiVersion"`
	Name       string     `yaml:"name"`
	Nodes      []kindNode `yaml:"nodes"`
}

type kindNode struct {
	Role string `yaml:"role"`
}

// CreateCluster creates a new kind cluster
func CreateCluster(ctx context.Context, cfg *config.ClusterConfig) error {
	// Check if kind is installed
	if err := checkKindInstalled(); err != nil {
		return err
	}

	// Check if cluster already exists
	exists, err := clusterExists(cfg.Name)
	if err != nil {
		return fmt.Errorf("failed to check if cluster exists: %w", err)
	}
	if exists {
		return fmt.Errorf("cluster '%s' already exists", cfg.Name)
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
	fmt.Printf("Creating kind cluster '%s'...\n", cfg.Name)
	cmd := exec.CommandContext(ctx, "kind", "create", "cluster",
		"--name", cfg.Name,
		"--config", configFile,
		"--wait", "2m")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create kind cluster: %w", err)
	}

	// Get kubeconfig path
	kubeconfigPath, err := getKubeconfigPath(cfg.Name)
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig path: %w", err)
	}

	// Save cluster metadata
	metadata := ClusterMetadata{
		Name:           cfg.Name,
		KubeconfigPath: kubeconfigPath,
		CreatedAt:      time.Now(),
	}
	if err := saveClusterMetadata(cfg.Name, &metadata); err != nil {
		return fmt.Errorf("failed to save cluster metadata: %w", err)
	}

	fmt.Printf("✓ Cluster '%s' created successfully\n", cfg.Name)
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

	// Remove cluster metadata
	if err := removeClusterMetadata(name); err != nil {
		// Log warning but don't fail
		fmt.Fprintf(os.Stderr, "Warning: failed to remove cluster metadata: %v\n", err)
	}

	fmt.Printf("✓ Cluster '%s' deleted successfully\n", name)
	return nil
}

// GetKubeconfigPath returns the kubeconfig path for a cluster
func GetKubeconfigPath(name string) (string, error) {
	return getKubeconfigPath(name)
}

// ListClusters lists all kind clusters managed by kueue-bench
func ListClusters() ([]ClusterMetadata, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	clustersDir := filepath.Join(home, metadataDir)
	if _, err := os.Stat(clustersDir); os.IsNotExist(err) {
		return []ClusterMetadata{}, nil
	}

	entries, err := os.ReadDir(clustersDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read clusters directory: %w", err)
	}

	var clusters []ClusterMetadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metadataPath := filepath.Join(clustersDir, entry.Name(), "metadata.json")
		data, err := os.ReadFile(metadataPath)
		if err != nil {
			continue // Skip if metadata can't be read
		}

		var metadata ClusterMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			continue // Skip if metadata is invalid
		}

		clusters = append(clusters, metadata)
	}

	return clusters, nil
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
		Name:       cfg.Name,
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

func getKubeconfigPath(name string) (string, error) {
	cmd := exec.Command("kind", "get", "kubeconfig", "--name", name)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Write kubeconfig to dedicated file
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	kubeconfigDir := filepath.Join(home, metadataDir, name)
	if err := os.MkdirAll(kubeconfigDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	kubeconfigPath := filepath.Join(kubeconfigDir, "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, output, 0600); err != nil {
		return "", fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	return kubeconfigPath, nil
}

func saveClusterMetadata(name string, metadata *ClusterMetadata) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	metadataPath := filepath.Join(home, metadataDir, name, "metadata.json")
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

func removeClusterMetadata(name string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	metadataPath := filepath.Join(home, metadataDir, name)
	return os.RemoveAll(metadataPath)
}
