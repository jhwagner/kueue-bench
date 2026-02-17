package cluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jhwagner/kueue-bench/pkg/config"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster"
)

var (
	provider     *cluster.Provider
	providerOnce sync.Once
)

// getProvider returns a shared kind cluster provider.
// A single provider is reused to avoid accumulating Docker client connections.
func getProvider() *cluster.Provider {
	providerOnce.Do(func() {
		provider = cluster.NewProvider()
	})
	return provider
}

// CreateCluster creates a new kind cluster
func CreateCluster(ctx context.Context, name string, cfg *config.ClusterConfig, kubeconfigPath string) error {
	provider := getProvider()

	// Check if cluster already exists
	clusters, err := provider.List()
	if err != nil {
		return fmt.Errorf("failed to list clusters: %w", err)
	}
	for _, c := range clusters {
		if c == name {
			return fmt.Errorf("cluster '%s' already exists", name)
		}
	}

	// Generate kind config
	kindConfig := generateKindConfig(cfg)

	// Create cluster
	fmt.Printf("Creating kind cluster '%s'...\n", name)
	if err := provider.Create(
		name,
		cluster.CreateWithV1Alpha4Config(kindConfig),
		cluster.CreateWithWaitForReady(2*time.Minute),
	); err != nil {
		return fmt.Errorf("failed to create kind cluster: %w", err)
	}

	// Export kubeconfig to specified path
	if err := ExportKubeconfig(name, kubeconfigPath); err != nil {
		return fmt.Errorf("failed to export kubeconfig: %w", err)
	}

	fmt.Printf("✓ Cluster '%s' created successfully\n", name)
	return nil
}

// DeleteCluster deletes a kind cluster
func DeleteCluster(ctx context.Context, name string) error {
	provider := getProvider()

	// Check if cluster exists
	clusters, err := provider.List()
	if err != nil {
		return fmt.Errorf("failed to list clusters: %w", err)
	}
	exists := false
	for _, c := range clusters {
		if c == name {
			exists = true
			break
		}
	}
	if !exists {
		return fmt.Errorf("cluster '%s' does not exist", name)
	}

	// Delete cluster
	fmt.Printf("Deleting kind cluster '%s'...\n", name)
	if err := provider.Delete(name, ""); err != nil {
		return fmt.Errorf("failed to delete kind cluster: %w", err)
	}

	fmt.Printf("✓ Cluster '%s' deleted successfully\n", name)
	return nil
}

// Helper functions

func generateKindConfig(_ *config.ClusterConfig) *v1alpha4.Cluster {
	kindCfg := &v1alpha4.Cluster{
		Nodes: []v1alpha4.Node{
			{Role: v1alpha4.ControlPlaneRole},
		},
	}

	return kindCfg
}

// ExportKubeconfig exports a kubeconfig for the given kind cluster to a file.
func ExportKubeconfig(name string, kubeconfigPath string) error {
	data, err := GetKubeconfig(name, false)
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(kubeconfigPath), 0750); err != nil {
		return fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	if err := os.WriteFile(kubeconfigPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	return nil
}

// GetKubeconfig returns the raw kubeconfig bytes for a kind cluster.
// When internal is true, uses the cluster's Docker network address instead of 127.0.0.1,
// which is needed for inter-cluster connectivity (e.g. MultiKueue management to worker).
func GetKubeconfig(name string, internal bool) ([]byte, error) {
	provider := getProvider()
	kubeconfig, err := provider.KubeConfig(name, internal)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	return []byte(kubeconfig), nil
}
