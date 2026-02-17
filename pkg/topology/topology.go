package topology

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jhwagner/kueue-bench/pkg/cluster"
	"github.com/jhwagner/kueue-bench/pkg/config"
	"github.com/jhwagner/kueue-bench/pkg/extensions"
	"github.com/jhwagner/kueue-bench/pkg/kueue"
	"github.com/jhwagner/kueue-bench/pkg/kwok"
)

const (
	metadataDir      = ".kueue-bench/topologies"
	metadataFilename = "metadata.json"
)

// Topology represents a Kueue test topology
type Topology struct {
	metadata *Metadata
}

// Create creates a new topology with all its clusters and components
func Create(ctx context.Context, name string, cfg *config.Topology) (t *Topology, err error) {
	t = &Topology{
		metadata: &Metadata{
			Name:      name,
			CreatedAt: time.Now(),
			Clusters:  make(map[string]Cluster),
		},
	}

	// Get topology directory for storing kubeconfigs
	topologyDir, err := getTopologyDir(name)
	if err != nil {
		return nil, err
	}

	// Create topology directory
	if err := os.MkdirAll(topologyDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create topology directory: %w", err)
	}

	// Track created clusters for cleanup on error
	var createdClusters []string

	// Cleanup on error
	defer func() {
		if err != nil {
			if len(createdClusters) > 0 {
				fmt.Fprintf(os.Stderr, "\nTopology creation failed, cleaning up %d cluster(s)...\n", len(createdClusters))
				for _, kindClusterName := range createdClusters {
					if err := cluster.DeleteCluster(ctx, kindClusterName); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: failed to cleanup cluster %s: %v\n", kindClusterName, err)
					}
				}
			}
			// Remove topology directory
			if err := os.RemoveAll(topologyDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove topology directory: %v\n", err)
			}
		}
	}()

	// Get Kwok version from spec
	kwokVersion := kwok.DefaultKwokVersion
	if cfg.Spec.Kwok != nil && cfg.Spec.Kwok.Version != "" {
		kwokVersion = cfg.Spec.Kwok.Version
	}

	// Get Kueue version and helm values from spec
	kueueVersion := kueue.DefaultKueueVersion
	var kueueHelmValues map[string]interface{}
	if cfg.Spec.Kueue != nil {
		if cfg.Spec.Kueue.Version != "" {
			kueueVersion = cfg.Spec.Kueue.Version
		}
		kueueHelmValues = cfg.Spec.Kueue.HelmValues
	}

	// Expand WorkerSets into worker ClusterConfigs
	expandedWorkers, err := config.ExpandWorkerSets(cfg.Spec.WorkerSets)
	if err != nil {
		return nil, fmt.Errorf("failed to expand worker sets: %w", err)
	}

	// Combine explicit clusters with expanded workers (new slice to avoid mutating cfg.Spec.Clusters)
	allClusters := make([]config.ClusterConfig, 0, len(cfg.Spec.Clusters)+len(expandedWorkers))
	allClusters = append(allClusters, cfg.Spec.Clusters...)
	allClusters = append(allClusters, expandedWorkers...)

	// Classify clusters by role in a single pass
	var managementCluster *config.ClusterConfig
	var workerClusters []*config.ClusterConfig
	var standaloneClusters []*config.ClusterConfig
	for i := range allClusters {
		switch allClusters[i].Role {
		case config.RoleManagement:
			managementCluster = &allClusters[i]
		case config.RoleWorker:
			workerClusters = append(workerClusters, &allClusters[i])
		default:
			standaloneClusters = append(standaloneClusters, &allClusters[i])
		}
	}

	// Create worker clusters first (with Kueue objects)
	for _, clusterCfg := range workerClusters {
		if err := t.createCluster(ctx, clusterCfg, topologyDir, kwokVersion, kueueVersion, kueueHelmValues, &createdClusters); err != nil {
			return nil, err
		}
	}

	// Create standalone clusters
	for _, clusterCfg := range standaloneClusters {
		if err := t.createCluster(ctx, clusterCfg, topologyDir, kwokVersion, kueueVersion, kueueHelmValues, &createdClusters); err != nil {
			return nil, err
		}
	}

	// Create management cluster (if exists)
	if managementCluster != nil {
		// Create cluster infrastructure (kind + Kwok + Kueue + extensions install, but no Kueue objects yet)
		kubeconfigPath, err := t.createClusterInfrastructure(ctx, managementCluster, topologyDir, kwokVersion, kueueVersion, kueueHelmValues, &createdClusters)
		if err != nil {
			return nil, err
		}

		// Create Kueue client for management cluster (used for MultiKueue setup and object provisioning)
		kueueClient, err := kueue.NewClient(kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create Kueue client for management cluster: %w", err)
		}

		// Setup MultiKueue infrastructure (if WorkerSets exist)
		if len(cfg.Spec.WorkerSets) > 0 {
			// Get internal kubeconfigs for inter-cluster connectivity
			// (default kubeconfigs use 127.0.0.1 which is unreachable from other kind containers)
			workerKubeconfigs := make(map[string][]byte, len(workerClusters))
			for _, worker := range workerClusters {
				kindClusterName := t.getKindClusterName(worker.Name)
				kubeconfigData, err := cluster.GetKubeconfig(kindClusterName, true)
				if err != nil {
					return nil, fmt.Errorf("failed to get internal kubeconfig for worker %q: %w", worker.Name, err)
				}
				workerKubeconfigs[worker.Name] = kubeconfigData
			}

			// Create MultiKueue infrastructure (Secrets, MultiKueueClusters, MultiKueueConfigs, AdmissionChecks)
			if err := kueue.SetupMultiKueueInfrastructure(ctx, kueueClient, cfg.Spec.WorkerSets, workerKubeconfigs); err != nil {
				return nil, fmt.Errorf("failed to setup MultiKueue infrastructure: %w", err)
			}
		}

		// Derive management Kueue objects from WorkerSets + user-defined config
		derivedConfig := config.DeriveManagementKueueConfig(cfg.Spec.WorkerSets, expandedWorkers, managementCluster.Kueue)

		// Provision management Kueue objects
		if derivedConfig != nil {
			if err := kueue.ProvisionKueueObjects(ctx, kueueClient, derivedConfig); err != nil {
				return nil, fmt.Errorf("failed to provision Kueue objects in management cluster: %w", err)
			}
		}
	}

	// Save metadata
	if err := t.save(); err != nil {
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	return t, nil
}

// createCluster creates a complete cluster with all components (infrastructure + Kueue objects)
func (t *Topology) createCluster(ctx context.Context, clusterCfg *config.ClusterConfig, topologyDir, kwokVersion, kueueVersion string, kueueHelmValues map[string]interface{}, createdClusters *[]string) error {
	kubeconfigPath, err := t.createClusterInfrastructure(ctx, clusterCfg, topologyDir, kwokVersion, kueueVersion, kueueHelmValues, createdClusters)
	if err != nil {
		return err
	}

	// Provision Kueue objects (if specified)
	if clusterCfg.Kueue != nil {
		kueueClient, err := kueue.NewClient(kubeconfigPath)
		if err != nil {
			return fmt.Errorf("failed to create Kueue client for cluster '%s': %w", clusterCfg.Name, err)
		}

		if err := kueue.ProvisionKueueObjects(ctx, kueueClient, clusterCfg.Kueue); err != nil {
			return fmt.Errorf("failed to provision Kueue objects in cluster '%s': %w", clusterCfg.Name, err)
		}
	}

	return nil
}

// createClusterInfrastructure creates cluster infrastructure (kind + Kwok + Kueue install) without Kueue objects
func (t *Topology) createClusterInfrastructure(ctx context.Context, clusterCfg *config.ClusterConfig, topologyDir, kwokVersion, kueueVersion string, kueueHelmValues map[string]interface{}, createdClusters *[]string) (string, error) {
	clusterName := clusterCfg.Name
	kindClusterName := t.getKindClusterName(clusterName)
	kubeconfigPath := filepath.Join(topologyDir, fmt.Sprintf("%s.kubeconfig", clusterName))

	// Create kind cluster
	if err := cluster.CreateCluster(ctx, kindClusterName, clusterCfg, kubeconfigPath); err != nil {
		return "", fmt.Errorf("failed to create cluster '%s': %w", clusterName, err)
	}
	// Track created cluster for cleanup on error
	*createdClusters = append(*createdClusters, kindClusterName)

	// Install Kwok
	if err := kwok.Install(ctx, kubeconfigPath, kwokVersion); err != nil {
		return "", fmt.Errorf("failed to install Kwok in cluster '%s': %w", clusterName, err)
	}

	// Create Kwok nodes
	if err := kwok.CreateNodes(ctx, kubeconfigPath, clusterCfg.NodePools); err != nil {
		return "", fmt.Errorf("failed to create nodes in cluster '%s': %w", clusterName, err)
	}

	// Install Kueue
	if err := kueue.Install(ctx, kubeconfigPath, kueueVersion, kueueHelmValues); err != nil {
		return "", fmt.Errorf("failed to install Kueue in cluster '%s': %w", clusterName, err)
	}

	// Install extensions (after Kueue install, before Kueue objects)
	if len(clusterCfg.Extensions) > 0 {
		if err := extensions.InstallExtensions(ctx, kubeconfigPath, clusterCfg.Extensions); err != nil {
			return "", fmt.Errorf("failed to install extensions in cluster '%s': %w", clusterName, err)
		}
	}

	// Add cluster to metadata
	t.metadata.Clusters[clusterName] = Cluster{
		Name:            clusterName,
		KindClusterName: kindClusterName,
		KubeconfigPath:  kubeconfigPath,
		CreatedAt:       time.Now(),
	}

	return kubeconfigPath, nil
}

// Load loads an existing topology from disk
func Load(name string) (*Topology, error) {
	topologyDir, err := getTopologyDir(name)
	if err != nil {
		return nil, err
	}

	metadataPath := filepath.Join(topologyDir, metadataFilename)
	data, err := os.ReadFile(metadataPath) //nolint:gosec // path is constructed from known base directory
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &Topology{
		metadata: &metadata,
	}, nil
}

// List lists all topologies from disk
func List() ([]*Topology, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	topologiesDir := filepath.Join(home, metadataDir)
	entries, err := os.ReadDir(topologiesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Topology{}, nil
		}
		return nil, fmt.Errorf("failed to read topologies directory: %w", err)
	}

	var topologies []*Topology
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		topo, err := Load(entry.Name())
		if err != nil {
			// Skip entries that fail to load
			continue
		}

		topologies = append(topologies, topo)
	}

	// Sort by creation time (newest first)
	sort.Slice(topologies, func(i, j int) bool {
		return topologies[i].metadata.CreatedAt.After(topologies[j].metadata.CreatedAt)
	})

	return topologies, nil
}

// Delete deletes the topology and all its clusters
func (t *Topology) Delete(ctx context.Context) error {
	// Delete all kind clusters (best effort - continue on errors)
	for _, clusterInfo := range t.metadata.Clusters {
		if err := cluster.DeleteCluster(ctx, clusterInfo.KindClusterName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete cluster %s: %v\n", clusterInfo.Name, err)
		}
	}

	// Delete metadata directory
	topologyDir, err := getTopologyDir(t.metadata.Name)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(topologyDir); err != nil {
		return fmt.Errorf("failed to remove topology directory: %w", err)
	}

	return nil
}

// GetMetadata returns the topology metadata
func (t *Topology) GetMetadata() *Metadata {
	return t.metadata
}

// save saves topology metadata to disk
func (t *Topology) save() error {
	topologyDir, err := getTopologyDir(t.metadata.Name)
	if err != nil {
		return err
	}

	metadataPath := filepath.Join(topologyDir, metadataFilename)
	data, err := json.MarshalIndent(t.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// getKindClusterName returns the kind cluster name for a cluster
func (t *Topology) getKindClusterName(clusterName string) string {
	return fmt.Sprintf("%s-%s", t.metadata.Name, clusterName)
}

// getTopologyDir returns the directory path for a topology
func getTopologyDir(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(home, metadataDir, name), nil
}
